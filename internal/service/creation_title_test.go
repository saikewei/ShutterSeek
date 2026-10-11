package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestCreationTitleNormalizationSupportsChineseNames(t *testing.T) {
	for _, tt := range []struct {
		input        string
		allowDefault bool
		want         string
		invalid      bool
	}{
		{"  海边记忆  ", false, "海边记忆", false},
		{strings.Repeat("风", 100), false, strings.Repeat("风", 100), false},
		{strings.Repeat("风", 101), false, "", true},
		{" \n\t", false, "", true},
		{" \n\t", true, "照片作品", false},
		{string([]byte{0xff}), false, "", true},
	} {
		got, err := normalizedCreationTitle(tt.input, tt.allowDefault)
		if tt.invalid {
			if !errors.Is(err, ErrCreationInvalid) {
				t.Fatalf("invalid name accepted: %v", err)
			}
		} else if err != nil || got != tt.want {
			t.Fatalf("normalization failed: %q %v", got, err)
		}
	}
}
func titleRequest() CreationTitleRequest {
	base := "照片作品"
	return CreationTitleRequest{Title: "  海边记忆  ", BaseTitle: &base}
}
func TestCreationRenameChangesOnlyOwnedMetadata(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2 .* FOR UPDATE`).
		WithArgs("work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "photo_ids", "state", "current_version"}).
		AddRow("work", 7, "照片作品", "[1,3]", `{"user_facts":[{"text":"private fact"}],"photo_edits":{"3":90}}`, "v1"))
	mock.ExpectExec(`UPDATE "creations" SET "title"=\$1,"updated_at"=\$2 WHERE "id" = \$3`).
		WithArgs("海边记忆", sqlmock.AnyArg(), "work").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := s.Rename(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", titleRequest()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationRenameRejectsOtherOwnerBeforeWriting(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2 .* FOR UPDATE`).
		WithArgs("other-work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if err := s.Rename(context.Background(), CreationActor{ID: 7, Role: "admin"}, "other-work", titleRequest()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationRenameRejectsStaleTitleBeforeWriting(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2 .* FOR UPDATE`).
		WithArgs("work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow("work", "new name"))
	mock.ExpectRollback()
	if err := s.Rename(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", titleRequest()); !errors.Is(err, ErrCreationConflict) {
		t.Fatalf("stale title accepted: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationRenameRollsBackOnWriteFailure(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2 .* FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow("work", "照片作品"))
	sentinel := errors.New("write unavailable")
	mock.ExpectExec(`UPDATE "creations"`).WillReturnError(sentinel)
	mock.ExpectRollback()
	if err := s.Rename(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", titleRequest()); !errors.Is(err, sentinel) {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationRenameRejectsInvalidInputWithoutDatabaseWrites(t *testing.T) {
	for _, req := range []CreationTitleRequest{{Title: "valid"}, {Title: "  ", BaseTitle: titleRequest().BaseTitle}, {Title: strings.Repeat("风", 101), BaseTitle: titleRequest().BaseTitle}} {
		s, mock := mockCreations(t)
		if err := s.Rename(context.Background(), CreationActor{ID: 7, Role: "admin"}, "work", req); !errors.Is(err, ErrCreationInvalid) {
			t.Fatalf("invalid name accepted: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
