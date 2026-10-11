package service

import (
	"context"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net"
	"shutterseek/internal/model"
	"strings"
	"testing"
	"time"
)

func mockCreations(t *testing.T) (*CreationService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	g, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return &CreationService{db: g}, mock
}
func TestCreationOwnerIsRequired(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("other-work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err := s.Owned(context.Background(), 7, "other-work")
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationCredentialIsBoundToActiveRun(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "creation_runs" WHERE id = \$1 AND credential_hash = \$2 AND expires_at > \$3 AND status = \$4`).WithArgs("run", credentialHash("wrong"), sqlmock.AnyArg(), "running", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	request, _ := structpb.NewStruct(map[string]interface{}{"run_id": "run", "credential": "wrong", "op": "context"})
	_, err := s.Call(context.Background(), request)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationPhotoScopeAndCheckpointBoundary(t *testing.T) {
	s, _ := mockCreations(t)
	run := model.CreationRun{ID: "run"}
	creation := model.Creation{PhotoIDs: "[1,2]"}
	_, err := s.callback(context.Background(), run, creation, "photo", map[string]interface{}{"id": float64(3)})
	if err == nil {
		t.Fatal("out-of-scope photo allowed")
	}
	_, err = s.callback(context.Background(), run, creation, "checkpoint_put", map[string]interface{}{"image": "data:image/png;base64,secret"})
	if err == nil {
		t.Fatal("image bytes stored in checkpoint")
	}
	_, err = s.callback(context.Background(), run, creation, "checkpoint_put", map[string]interface{}{"credential": "secret"})
	if err == nil {
		t.Fatal("credential stored in checkpoint")
	}
}
func TestCreationVerificationRequiresEvidence(t *testing.T) {
	good := map[string]interface{}{"rounds": float64(1), "screenshots": []interface{}{map[string]interface{}{"id": "a", "viewport": "desktop"}, map[string]interface{}{"id": "b", "viewport": "mobile"}}, "issues": []interface{}{}, "browser_ok": true, "visual_reviewed": true}
	if !validCreationReport(good) {
		t.Fatal("valid report rejected")
	}
	for _, key := range []string{"browser_ok", "visual_reviewed"} {
		copy := mapText(jsonText(good))
		copy[key] = false
		if validCreationReport(copy) {
			t.Fatalf("%s ignored", key)
		}
	}
	good["rounds"] = float64(4)
	if validCreationReport(good) {
		t.Fatal("repair limit ignored")
	}
	good["rounds"] = float64(3)
	good["issues"] = []interface{}{map[string]interface{}{"severity": "blocking"}}
	if validCreationReport(good) {
		t.Fatal("blocking defect accepted")
	}
}
func TestCreationLimitsBeforeDatabaseAccess(t *testing.T) {
	s := &CreationService{}
	_, err := s.Create(context.Background(), 1, CreationRequest{PhotoIDs: make([]int64, 201)})
	if err != ErrCreationInvalid {
		t.Fatal(err)
	}
	_, err = s.Run(context.Background(), 1, "work", CreationRunRequest{Action: "execute"})
	if err != ErrCreationInvalid {
		t.Fatal(err)
	}
	if strings.Contains(CreationCSP, "allow-same-origin") || !strings.Contains(CreationCSP, "connect-src 'none'") {
		t.Fatal("unsafe sandbox policy")
	}
	if credentialHash("secret") == credentialHash("different") {
		t.Fatal("credential hashes collide")
	}
}

func expectCreationForRun(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "current_version", "state", "photo_ids"}).AddRow("work", 7, "", `{"proposal":{}}`, "[1,2]"))
}
func TestCreationActiveUserAndGlobalLimits(t *testing.T) {
	for _, test := range []struct {
		name        string
		userCount   int64
		globalCount int64
	}{{"one per user", 1, 0}, {"five global", 0, 5}} {
		t.Run(test.name, func(t *testing.T) {
			s, mock := mockCreations(t)
			expectCreationForRun(mock)
			mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.userCount))
			if test.userCount == 0 {
				mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.globalCount))
			}
			mock.ExpectRollback()
			_, err := s.Run(context.Background(), 7, "work", CreationRunRequest{Action: "answer"})
			if err != ErrCreationConflict {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCreationConfirmedSelectionCannotEscapeScope(t *testing.T) {
	s, mock := mockCreations(t)
	expectCreationForRun(mock)
	mock.ExpectRollback()
	_, err := s.Run(context.Background(), 7, "work", CreationRunRequest{Action: "confirm", Proposal: &CreationProposal{Structure: "story", Style: "clean", UsedPhotoIDs: []int64{999}}})
	if err != ErrCreationInvalid {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationScreenshotUsesOwnerAndWorkScope(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).AddRow("work", 7))
	mock.ExpectQuery(`SELECT .* FROM "creation_assets" JOIN creation_runs.*creation_assets.id = \$1 AND creation_runs.creation_id = \$2`).WithArgs("other-asset", "work", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := s.Screenshot(context.Background(), 7, "work", "other-asset"); err != gorm.ErrRecordNotFound {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationCancelRevokesRunAndStopsExecution(t *testing.T) {
	s, mock := mockCreations(t)
	called := false
	s.cancel = map[string]context.CancelFunc{"run": func() { called = true }}
	mock.ExpectQuery(`SELECT .* FROM "creation_runs" WHERE id = \$1 AND creation_id = \$2 AND user_id = \$3`).WithArgs("run", "work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("run", "running"))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "creation_runs"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := s.Cancel(context.Background(), 7, "work", "run"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("running RPC not cancelled")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreationPreviewBlocksSelfNavigationAndKeepsOpaqueFrame(t *testing.T) {
	if !strings.Contains(CreationCSP, "frame-src about:") {
		t.Fatal("untrusted frame can navigate to the network")
	}
	doc := CreationPreviewDocument("<!doctype html><script>parent.document.cookie</script>")
	if !strings.Contains(doc, `sandbox="allow-scripts"`) || !strings.Contains(doc, "srcdoc=") || !strings.Contains(doc, "&lt;script&gt;") {
		t.Fatal("generated code escaped the nested sandbox")
	}
	if strings.Contains(doc, "allow-same-origin") {
		t.Fatal("opaque origin was removed")
	}
}

func TestCreationRestoreChecksBaseAndRestoresSnapshot(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "current_version"}).AddRow("work", 7, "v2"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE id = \$1 AND creation_id = \$2`).WithArgs("v1", "work", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "state"}).AddRow("v1", "work", `{"proposal":{"style":"restored"}}`))
	mock.ExpectExec(`UPDATE "creations"`).WithArgs("v1", `{"proposal":{"style":"restored"}}`, sqlmock.AnyArg(), "work").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := s.Restore(context.Background(), 7, "work", "v1", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreationEmptyStartupAndCleanupDoNotWrite(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT "id" FROM "creation_runs" WHERE status = \$1`).WithArgs("running").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := s.recoverCreationRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT "creation_assets"\."id" FROM "creation_assets" JOIN creation_runs`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := s.cleanupCreationAssets(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreationContextRPCEncodesPhotosAndMessages(t *testing.T) {
	for _, withMessage := range []bool{false, true} {
		t.Run(fmt.Sprint(withMessage), func(t *testing.T) {
			s, mock := mockCreations(t)
			mock.ExpectQuery(`SELECT .* FROM "creation_runs" WHERE id = \$1 AND credential_hash = \$2 AND expires_at > \$3 AND status = \$4`).WithArgs("run", credentialHash("test-capability"), sqlmock.AnyArg(), "running", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "user_id", "input"}).
					AddRow("run", "work", 7, `{"action":"answer","message":"A trip"}`))
			mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("work", int64(7), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "photo_ids", "state"}).
					AddRow("work", 7, "Travel", "[1,2]", `{"observations":[]}`))
			rows := sqlmock.NewRows([]string{"id", "creation_id", "role", "content", "created_at"})
			if withMessage {
				rows.AddRow("message", "work", "user", "A trip", time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC))
			}
			mock.ExpectQuery(`SELECT .* FROM "creation_messages" WHERE creation_id = \$1`).WithArgs("work", 40).WillReturnRows(rows)
			request, _ := structpb.NewStruct(map[string]interface{}{"run_id": "run", "credential": "test-capability", "op": "context"})
			out, err := s.Call(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			value := out.AsMap()
			photos := value["photo_ids"].([]interface{})
			if len(photos) != 2 || photos[0] != float64(1) || photos[1] != float64(2) {
				t.Fatalf("invalid photos: %v", photos)
			}
			messages, ok := value["messages"].([]interface{})
			if !ok {
				t.Fatal("messages must be a JSON array")
			}
			if withMessage {
				if len(messages) != 1 {
					t.Fatalf("invalid messages: %v", messages)
				}
				message := messages[0].(map[string]interface{})
				if message["role"] != "user" || message["content"] != "A trip" || message["created_at"] != "2026-10-11T08:00:00Z" {
					t.Fatalf("invalid message: %v", message)
				}
			} else if len(messages) != 0 {
				t.Fatalf("unexpected messages: %v", messages)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreationFailureReasonPersistsAfterAgentAbort(t *testing.T) {
	s, mock := mockCreations(t)
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "shutterseek.creation.v1.Agent",
		HandlerType: (*interface{})(nil),
		Streams: []grpc.StreamDesc{{StreamName: "Execute", ServerStreams: true, Handler: func(_ interface{}, stream grpc.ServerStream) error {
			request := new(structpb.Struct)
			if err := stream.RecvMsg(request); err != nil {
				return err
			}
			event, _ := structpb.NewStruct(map[string]interface{}{"stage": "failed", "error_code": "confirmed_photos_changed"})
			if err := stream.SendMsg(event); err != nil {
				return err
			}
			return status.Error(codes.Internal, "Agent execution failed; see server diagnostics.")
		}}},
	}, struct{}{})
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///creation-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	s.conn = conn
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "creation_runs"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	for _, id := range []int64{1, 2} {
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "creation_events"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
		mock.ExpectCommit()
	}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "creation_runs"`).
		WithArgs("", jsonText(creationFailureReport("confirmed_photos_changed")), "failed", sqlmock.AnyArg(), "run", "running").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.execute(ctx, model.CreationRun{ID: "run", CreatedAt: time.Now()})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	report := creationFailureReport("private-invalid-reason")
	if report["error_code"] != "agent_execution_failed" || strings.Contains(report["error"].(string), "private") {
		t.Fatal("unknown failure reason leaked")
	}
}

func TestCreationDetailRetainsConfirmedProposalWithoutExposingRunInput(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("work", int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "photo_ids", "state"}).AddRow("work", 7, "[1,2]", `{"proposal":{"used_photo_ids":[1,2]}}`))
	mock.ExpectQuery(`SELECT .* FROM "creation_messages" WHERE creation_id = \$1`).WithArgs("work").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE creation_id = \$1`).WithArgs("work").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT .* FROM "creation_runs" WHERE creation_id = \$1`).WithArgs("work", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "action", "status", "input", "credential_hash"}).
			AddRow("run", "confirm", "failed", `{"message":"private-run-input","proposal":{"structure":"Story","style":"Clean","used_photo_ids":[1]}}`, "private-run-credential"))
	detail, err := s.Detail(context.Background(), 7, "work")
	if err != nil {
		t.Fatal(err)
	}
	views := detail["runs"].([]CreationRunView)
	if len(views) != 1 || views[0].Proposal == nil || len(views[0].Proposal.UsedPhotoIDs) != 1 || views[0].Proposal.UsedPhotoIDs[0] != 1 {
		t.Fatal("confirmed selection was lost")
	}
	encoded := jsonText(detail)
	if strings.Contains(encoded, "private-run-input") || strings.Contains(encoded, "private-run-credential") {
		t.Fatal("private run fields exposed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreationChineseUpdateStillRequiresOwnership(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("other", int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	_, err := s.Run(context.Background(), 7, "other", CreationRunRequest{Action: "localize"})
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("unexpected ownership result: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
