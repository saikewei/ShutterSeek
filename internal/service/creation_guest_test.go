package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

func guestCreator() CreationActor { return CreationActor{ID: 7, Role: "guest"} }
func expectGuestPhotoCount(mock sqlmock.Sqlmock, ids []int64, count int64) {
	expectation := mock.ExpectQuery(`SELECT count\(\*\) FROM "photos" WHERE id IN .* AND id IN \(SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true\)`)
	switch len(ids) {
	case 1:
		expectation.WithArgs(ids[0])
	case 2:
		expectation.WithArgs(ids[0], ids[1])
	case 3:
		expectation.WithArgs(ids[0], ids[1], ids[2])
	}
	expectation.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}
func guestOwned(mock sqlmock.Sqlmock, ids, state string, lock bool) {
	query := `SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`
	if lock {
		query += ` .* FOR UPDATE`
	}
	mock.ExpectQuery(query).WithArgs("work", int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "photo_ids", "state", "current_version"}).AddRow("work", 7, ids, state, "v1"))
}
func TestGuestCreationRejectsPrivatePhotoIDsBeforeWriting(t *testing.T) {
	s, mock := mockCreations(t)
	expectGuestPhotoCount(mock, []int64{1, 2}, 1)
	_, err := s.Create(context.Background(), guestCreator(), CreationRequest{PhotoIDs: []int64{1, 2}, Title: "My work"})
	if !errors.Is(err, ErrCreationPhotoForbidden) {
		t.Fatalf("private scope accepted: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestCreationRejectsPrivateAlbumInSQL(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "album_photos" JOIN albums ON albums.id = album_photos.album_id WHERE album_photos.album_id = \$1 AND albums.is_public = true`).
		WithArgs(int64(12), 201).WillReturnRows(sqlmock.NewRows([]string{"photo_id"}))
	_, err := s.Create(context.Background(), guestCreator(), CreationRequest{AlbumID: 12})
	if !errors.Is(err, ErrCreationInvalid) {
		t.Fatalf("private album accepted: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestPhotoUpdateCannotAddPrivatePhoto(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	guestOwned(mock, "[1]", `{"photo_history":[1],"photo_revision":0}`, true)
	expectGuestPhotoCount(mock, []int64{1}, 1)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectGuestPhotoCount(mock, []int64{1, 2}, 1)
	mock.ExpectRollback()
	err := s.UpdatePhotos(context.Background(), guestCreator(), "work", CreationPhotosRequest{PhotoIDs: []int64{1, 2}, UsedPhotoIDs: []int64{1}, BaseVersion: "v1"})
	if !errors.Is(err, ErrCreationPhotoForbidden) {
		t.Fatalf("private photo added: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestRevocationBlocksContextPreviewScreenshotAndRun(t *testing.T) {
	for _, operation := range []string{"detail", "preview", "screenshot", "run"} {
		t.Run(operation, func(t *testing.T) {
			s, mock := mockCreations(t)
			if operation == "run" {
				mock.ExpectBegin()
				mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			guestOwned(mock, "[1]", `{"photo_history":[1,2]}`, operation == "run")
			expectGuestPhotoCount(mock, []int64{1, 2}, 1)
			if operation == "run" {
				mock.ExpectRollback()
			}
			var err error
			switch operation {
			case "detail":
				_, err = s.Detail(context.Background(), guestCreator(), "work")
			case "preview":
				_, err = s.Preview(context.Background(), guestCreator(), "work", "v1")
			case "screenshot":
				_, err = s.Screenshot(context.Background(), guestCreator(), "work", "shot")
			case "run":
				_, err = s.Run(context.Background(), guestCreator(), "work", CreationRunRequest{Action: "answer", BaseVersion: "v1"})
			}
			if !errors.Is(err, ErrCreationPhotoForbidden) {
				t.Fatalf("revoked photo exposed: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestGuestCannotRestoreVersionWithPrivatePhotos(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	guestOwned(mock, "[1]", `{"photo_history":[1]}`, true)
	expectGuestPhotoCount(mock, []int64{1}, 1)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE id = \$1 AND creation_id = \$2`).WithArgs("old", "work", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "used_photo_ids", "state"}).AddRow("old", "[2]", `{"photo_ids":[2]}`))
	expectGuestPhotoCount(mock, []int64{2}, 0)
	mock.ExpectRollback()
	if err := s.Restore(context.Background(), guestCreator(), "work", "old", "v1"); !errors.Is(err, ErrCreationPhotoForbidden) {
		t.Fatalf("private version restored: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestRPCCannotForgeAdminRoleAfterPhotoRevocation(t *testing.T) {
	for _, op := range []string{"context", "photo", "asset_get", "checkpoint_get", "finish"} {
		t.Run(op, func(t *testing.T) {
			s, mock := mockCreations(t)
			mock.ExpectQuery(`SELECT .* FROM "creation_runs" WHERE id = \$1 AND credential_hash = \$2 AND expires_at > \$3 AND status = \$4`).
				WithArgs("run", credentialHash("test-capability"), sqlmock.AnyArg(), "running", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "creation_id"}).AddRow("run", 7, "work"))
			mock.ExpectQuery(`SELECT "role" FROM "users" WHERE id = \$1`).WithArgs(int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("guest"))
			guestOwned(mock, "[1]", `{"photo_history":[1]}`, false)
			expectGuestPhotoCount(mock, []int64{1}, 0)
			input, _ := structpb.NewStruct(map[string]interface{}{"run_id": "run", "credential": "test-capability", "op": op, "role": "admin", "payload": map[string]interface{}{"id": float64(1), "role": "admin"}})
			_, err := s.Call(context.Background(), input)
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("callback ignored revocation: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestGuestCannotReadAnotherUsersWork(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("other", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := s.Detail(context.Background(), guestCreator(), "other"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user read allowed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationActorMissingOrUnknownRoleFailsClosed(t *testing.T) {
	for _, actor := range []CreationActor{{ID: 7}, {ID: 7, Role: "owner"}, {Role: "admin"}} {
		s, mock := mockCreations(t)
		if _, err := s.Create(context.Background(), actor, CreationRequest{PhotoIDs: []int64{1}}); !errors.Is(err, ErrCreationPhotoForbidden) {
			t.Fatalf("unknown actor allowed: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
