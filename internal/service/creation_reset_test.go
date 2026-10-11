package service

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

type resetStateArgument struct{}

func (resetStateArgument) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	state := mapText(text)
	return len(state) == 4 && photoRevision(state) == 3 &&
		jsonText(state["photo_ids"]) == "[1,3]" && jsonText(state["selected_photo_ids"]) == "[3]" &&
		jsonText(photoRotations(state)) == `{"3":90}`
}

func resetOwnedRow(mock sqlmock.Sqlmock, state string) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2 .* FOR UPDATE`).
		WithArgs("work", int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "photo_ids", "current_version", "state"}).
			AddRow("work", 7, "[1,3]", "v1", state))
}
func resetIdle(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs" WHERE creation_id = \$1 AND status IN \(\$2,\$3\)`).
		WithArgs("work", "queued", "running").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}
func resetDeletes(mock sqlmock.Sqlmock) {
	for _, table := range []string{"creation_checkpoints", "creation_assets", "creation_events"} {
		query := `DELETE FROM "` + table + `" WHERE run_id IN (SELECT "id" FROM "creation_runs" WHERE creation_id = $1)`
		mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs("work").WillReturnResult(sqlmock.NewResult(0, 2))
	}
	for _, table := range []string{"creation_versions", "creation_messages", "creation_runs"} {
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "` + table + `" WHERE creation_id = $1`)).
			WithArgs("work").WillReturnResult(sqlmock.NewResult(0, 2))
	}
}
func resetRequest() CreationResetRequest {
	revision := 2
	return CreationResetRequest{BaseVersion: "v1", BaseRevision: &revision}
}
func TestCreationResetPreservesOnlyPhotosSelectionAndRotation(t *testing.T) {
	s, mock := mockCreations(t)
	resetOwnedRow(mock, `{"photo_revision":2,"selected_photo_ids":[3],"photo_edits":{"3":90},"proposal":{"used_photo_ids":[1]},"observations":[{"photo_id":3}],"user_facts":[{"text":"private fact"}],"questions":["old question"],"photos_changed":true,"unknowns":["old unknown"],"reply":"old reply","html":"old page"}`)
	resetIdle(mock)
	resetDeletes(mock)
	mock.ExpectExec(`UPDATE "creations" SET "current_version"=\$1,"state"=\$2,"updated_at"=\$3 WHERE "id" = \$4`).
		WithArgs("", resetStateArgument{}, sqlmock.AnyArg(), "work").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := s.Reset(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", resetRequest()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationResetPreservesLegacyConfirmedSelection(t *testing.T) {
	s, mock := mockCreations(t)
	resetOwnedRow(mock, `{"photo_revision":2,"photo_edits":{"3":90},"proposal":{"used_photo_ids":[3],"structure":"old plan"}}`)
	resetIdle(mock)
	resetDeletes(mock)
	mock.ExpectExec(`UPDATE "creations"`).WithArgs("", resetStateArgument{}, sqlmock.AnyArg(), "work").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := s.Reset(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", resetRequest()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationResetRejectsOtherOwnerBeforeDeleting(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2 .* FOR UPDATE`).
		WithArgs("other-work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if err := s.Reset(context.Background(), CreationActor{ID: 7, Role: "admin"}, "other-work", resetRequest()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationResetRejectsStaleAndActiveWorkBeforeDeleting(t *testing.T) {
	for _, name := range []string{"stale-version", "stale-revision", "active-run"} {
		t.Run(name, func(t *testing.T) {
			s, mock := mockCreations(t)
			resetOwnedRow(mock, `{"photo_revision":2}`)
			req := resetRequest()
			switch name {
			case "stale-version":
				req.BaseVersion = "old"
			case "stale-revision":
				*req.BaseRevision = 1
			case "active-run":
				mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs" WHERE creation_id = \$1 AND status IN \(\$2,\$3\)`).
					WithArgs("work", "queued", "running").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			}
			mock.ExpectRollback()
			if err := s.Reset(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", req); !errors.Is(err, ErrCreationConflict) {
				t.Fatalf("unsafe reset allowed: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCreationResetRollsBackHistoryDeletionOnFailure(t *testing.T) {
	s, mock := mockCreations(t)
	resetOwnedRow(mock, `{"photo_revision":2}`)
	resetIdle(mock)
	resetDeletes(mock)
	sentinel := errors.New("write unavailable")
	mock.ExpectExec(`UPDATE "creations"`).WillReturnError(sentinel)
	mock.ExpectRollback()
	if err := s.Reset(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", resetRequest()); !errors.Is(err, sentinel) {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationResetRequiresExplicitRevision(t *testing.T) {
	for _, req := range []CreationResetRequest{{}, {BaseRevision: func() *int { n := -1; return &n }()}} {
		s, mock := mockCreations(t)
		if err := s.Reset(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", req); !errors.Is(err, ErrCreationInvalid) {
			t.Fatalf("missing guard accepted: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
