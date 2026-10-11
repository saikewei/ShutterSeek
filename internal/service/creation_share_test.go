package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func sharedFixture(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .* FROM "creation_shares" WHERE id = \$1 AND enabled = \$2`).WithArgs("share", true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "version_id", "enabled"}).AddRow("share", "admin-work", "v1", true))
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1`).WithArgs("admin-work", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "photo_ids", "state"}).AddRow("admin-work", 1, "Shared story", "[1,2]", `{"user_facts":[{"text":"private context"}]}`))
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE id = \$1 AND creation_id = \$2`).WithArgs("v1", "admin-work", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "used_photo_ids", "html", "report", "state"}).
			AddRow("v1", "admin-work", "[1]", `<img src="ss-photo:1"><img src="ss-photo:2"><button onclick="this.textContent='Done'">Open</button>`, `{"screenshots":[{"id":"private-shot"}]}`, `{"photo_edits":{}}`))
}
func TestGuestShareMetadataOmitsContextReportsAndOwner(t *testing.T) {
	s, mock := mockCreations(t)
	s.sharingEnabled = true
	sharedFixture(mock)
	expectGuestPhotoCount(mock, []int64{1}, 1)
	out, err := s.Shared(context.Background(), guestCreator(), "share")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	for _, forbidden := range []string{"user_facts", "private context", "screenshots", "private-shot", "user_id", "photo_ids", "html"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("shared metadata leaked %s", forbidden)
		}
	}
	if out.Title != "Shared story" || out.VersionID != "v1" {
		t.Fatal("wrong shared version")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestSharedPreviewEmbedsOnlyAuthorizedSelectedPhotos(t *testing.T) {
	directory, _ := sourcePhoto(t)
	s, mock := mockCreations(t)
	s.thumbnails = directory
	s.sharingEnabled = true
	sharedFixture(mock)
	expectGuestPhotoCount(mock, []int64{1}, 1)
	out, err := s.SharedPreview(context.Background(), guestCreator(), "share")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "data:image/png;base64,") || strings.Contains(out, "ss-photo:2") || !strings.Contains(out, `sandbox="allow-scripts"`) {
		t.Fatal("shared preview scope or sandbox changed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestShareReadBlocksPrivateOrRevokedSelectedPhoto(t *testing.T) {
	for _, preview := range []bool{false, true} {
		s, mock := mockCreations(t)
		s.sharingEnabled = true
		sharedFixture(mock)
		expectGuestPhotoCount(mock, []int64{1}, 0)
		var err error
		if preview {
			_, err = s.SharedPreview(context.Background(), guestCreator(), "share")
		} else {
			_, err = s.Shared(context.Background(), guestCreator(), "share")
		}
		if !errors.Is(err, ErrCreationPhotoForbidden) {
			t.Fatalf("private shared image exposed: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRevokedShareDoesNotRevealAnyWorkData(t *testing.T) {
	s, mock := mockCreations(t)
	s.sharingEnabled = true
	mock.ExpectQuery(`SELECT .* FROM "creation_shares" WHERE id = \$1 AND enabled = \$2`).WithArgs("closed", true, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := s.Shared(context.Background(), guestCreator(), "closed"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("revoked link exposed work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestShareControlRequiresOwnershipForEveryRole(t *testing.T) {
	for _, role := range []string{"guest", "admin"} {
		for _, op := range []string{"get", "enable", "revoke"} {
			t.Run(role+"/"+op, func(t *testing.T) {
				s, mock := mockCreations(t)
				s.sharingEnabled = true
				if op != "get" {
					mock.ExpectBegin()
				}
				mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).WithArgs("other", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
				if op != "get" {
					mock.ExpectRollback()
				}
				actor := CreationActor{ID: 7, Role: role}
				var err error
				switch op {
				case "get":
					_, err = s.OwnerShare(context.Background(), actor, "other")
				case "enable":
					_, err = s.EnableShare(context.Background(), actor, "other", CreationShareRequest{VersionID: "v1", BaseVersion: "v1"})
				case "revoke":
					err = s.RevokeShare(context.Background(), actor, "other")
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					t.Fatalf("share control crossed ownership: %v", err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestShareSchemaNotReadyDoesNotBreakExistingCreationService(t *testing.T) {
	s, mock := mockCreations(t)
	if s.SharingEnabled() {
		t.Fatal("sharing enabled without schema")
	}
	if _, err := s.Shared(context.Background(), guestCreator(), "share"); !errors.Is(err, ErrCreationSharingUnavailable) {
		t.Fatalf("missing schema accepted: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGuestCanCreateAuthenticatedShareForOwnPublicVersion(t *testing.T) {
	s, mock := mockCreations(t)
	s.sharingEnabled = true
	mock.ExpectBegin()
	guestOwned(mock, "[1]", `{"photo_history":[1]}`, true)
	expectGuestPhotoCount(mock, []int64{1}, 1)
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE id = \$1 AND creation_id = \$2`).WithArgs("v1", "work", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "used_photo_ids"}).AddRow("v1", "work", "[1]"))
	expectGuestPhotoCount(mock, []int64{1}, 1)
	mock.ExpectQuery(`SELECT .* FROM "creation_shares" WHERE creation_id = \$1`).WithArgs("work", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO "creation_shares"`).WithArgs(sqlmock.AnyArg(), "work", "v1", true, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	share, err := s.EnableShare(context.Background(), guestCreator(), "work", CreationShareRequest{VersionID: "v1", BaseVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if share.ID == "" || !share.Enabled || share.VersionID != "v1" {
		t.Fatal("share was not persisted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestUpdatingShareKeepsItsLinkAndSelectedVersion(t *testing.T) {
	s, mock := mockCreations(t)
	s.sharingEnabled = true
	mock.ExpectBegin()
	guestOwned(mock, "[1]", `{"photo_history":[1]}`, true)
	expectGuestPhotoCount(mock, []int64{1}, 1)
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE id = \$1 AND creation_id = \$2`).WithArgs("v1", "work", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "used_photo_ids"}).AddRow("v1", "work", "[1]"))
	expectGuestPhotoCount(mock, []int64{1}, 1)
	mock.ExpectQuery(`SELECT .* FROM "creation_shares" WHERE creation_id = \$1`).WithArgs("work", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "version_id", "enabled"}).AddRow("stable-share", "work", "old", false))
	mock.ExpectExec(`UPDATE "creation_shares" SET "enabled"=\$1,"updated_at"=\$2,"version_id"=\$3 WHERE "id" = \$4`).
		WithArgs(true, sqlmock.AnyArg(), "v1", "stable-share").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	share, err := s.EnableShare(context.Background(), guestCreator(), "work", CreationShareRequest{VersionID: "v1", BaseVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if share.ID != "stable-share" || !share.Enabled || share.VersionID != "v1" {
		t.Fatal("share link changed unexpectedly")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestGuestCanRevokeOwnShareAfterPhotoAccessIsRevoked(t *testing.T) {
	s, mock := mockCreations(t)
	s.sharingEnabled = true
	mock.ExpectBegin()
	guestOwned(mock, "[1]", `{"photo_history":[1,2]}`, true)
	mock.ExpectExec(`UPDATE "creation_shares" SET "enabled"=\$1,"updated_at"=\$2 WHERE creation_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), "work").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := s.RevokeShare(context.Background(), guestCreator(), "work"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
