package service

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/base64"
	"github.com/DATA-DOG/go-sqlmock"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"shutterseek/internal/model"
	"testing"
)

type photoStateArgument struct{ revision int }

func (a photoStateArgument) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	state := mapText(text)
	obs, ok := state["observations"].([]interface{})
	return photoRevision(state) == a.revision && jsonText(state["photo_ids"]) == "[1,3]" &&
		jsonText(state["selected_photo_ids"]) == "[3]" && photoRotations(state)["3"] == 90 &&
		state["photos_changed"] == true && jsonText(state["user_facts"]) == `[{"text":"private fact"}]` && ok && len(obs) == 1 &&
		obs[0].(map[string]interface{})["photo_id"] == float64(1)
}
func TestCreationPhotoUpdatePreservesFactsAndInvalidatesOnlyChangedPhotos(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).
		WithArgs("work", int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "current_version", "state"}).
		AddRow("work", 7, "v1", `{"photo_revision":2,"observations":[{"photo_id":1},{"photo_id":2}],"proposal":{"used_photo_ids":[1,2]},"user_facts":[{"text":"private fact"}]}`))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "photos"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec(`UPDATE "creations"`).WithArgs("[1,3]", photoStateArgument{3}, sqlmock.AnyArg(), "work").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := s.UpdatePhotos(context.Background(), 7, "work", CreationPhotosRequest{PhotoIDs: []int64{1, 3}, UsedPhotoIDs: []int64{3}, Rotations: map[string]int{"3": 90}, BaseVersion: "v1", BaseRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationPhotoUpdateRejectsStaleEditsBeforeAnyWrite(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "current_version", "state"}).AddRow("work", "v1", `{"photo_revision":2}`))
	mock.ExpectRollback()
	err := s.UpdatePhotos(context.Background(), 7, "work", CreationPhotosRequest{PhotoIDs: []int64{1}, UsedPhotoIDs: []int64{1}, BaseVersion: "v1", BaseRevision: 1})
	if err != ErrCreationConflict {
		t.Fatalf("stale write allowed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationPhotoUpdateRejectsActiveRunBeforeAnyWrite(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "current_version", "state"}).AddRow("work", "v1", "{}"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "creation_runs"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	err := s.UpdatePhotos(context.Background(), 7, "work", CreationPhotosRequest{PhotoIDs: []int64{1}, UsedPhotoIDs: []int64{1}, BaseVersion: "v1"})
	if err != ErrCreationConflict {
		t.Fatalf("active scope changed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationPhotoSettingsRejectInvalidScopeAndRotation(t *testing.T) {
	for _, req := range []CreationPhotosRequest{
		{PhotoIDs: []int64{1}, UsedPhotoIDs: []int64{2}},
		{PhotoIDs: []int64{1, 1}, UsedPhotoIDs: []int64{1}},
		{PhotoIDs: []int64{1}, UsedPhotoIDs: []int64{1}, Rotations: map[string]int{"2": 90}},
		{PhotoIDs: []int64{1}, UsedPhotoIDs: []int64{1}, Rotations: map[string]int{"1": 45}},
		{PhotoIDs: []int64{1}, UsedPhotoIDs: []int64{1}, Rotations: map[string]int{"01": 90}},
	} {
		if _, err := creationPhotoSettings(req.PhotoIDs, req.UsedPhotoIDs, req.Rotations); err != ErrCreationInvalid {
			t.Fatal("invalid photo settings accepted")
		}
	}
}
func sourcePhoto(t *testing.T) (string, []byte) {
	t.Helper()
	directory := t.TempDir()
	source := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			c := color.RGBA{R: 240, A: 255}
			if y >= 8 {
				c = color.RGBA{B: 240, A: 255}
			}
			source.Set(x, y, c)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "1.webp"), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return directory, data.Bytes()
}
func TestCreationRotatesEphemeralAssetWithoutChangingSource(t *testing.T) {
	directory, original := sourcePhoto(t)
	s := &CreationService{thumbnails: directory}
	for _, angle := range []int{90, 180, 270} {
		b, mime, err := s.creationPhotoBytes(1, angle)
		if err != nil {
			t.Fatal(err)
		}
		if mime != "image/jpeg" {
			t.Fatal("wrong asset media type")
		}
		rotated, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		w, h := 32, 16
		if angle != 180 {
			w, h = 16, 32
		}
		if rotated.Bounds().Dx() != w || rotated.Bounds().Dy() != h {
			t.Fatal("rotation dimensions changed incorrectly")
		}
		red, _, blue, _ := rotated.At(1, 1).RGBA()
		if (angle == 90 || angle == 180) && blue <= red {
			t.Fatal("clockwise rotation did not move the lower band correctly")
		}
	}
	after, _ := os.ReadFile(filepath.Join(directory, "1.webp"))
	if !bytes.Equal(after, original) {
		t.Fatal("source photo was modified")
	}
}
func TestCreationHistoricalPreviewUsesVersionScopeAndRotation(t *testing.T) {
	directory, _ := sourcePhoto(t)
	s, mock := mockCreations(t)
	s.thumbnails = directory
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "photo_ids", "current_version"}).AddRow("work", 7, "[2]", "v2"))
	mock.ExpectQuery(`SELECT .* FROM "creation_versions" WHERE id = \$1 AND creation_id = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creation_id", "html", "used_photo_ids", "state"}).
			AddRow("v1", "work", `<img src="ss-photo:1">`, "[1]", `{"photo_ids":[1],"photo_edits":{"1":90}}`))
	html, err := s.Preview(context.Background(), 7, "work", "v1")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`data:image/jpeg;base64,([A-Za-z0-9+/=]+)`).FindStringSubmatch(html)
	if len(match) != 2 {
		t.Fatal("removed historical photo lost its rotated asset")
	}
	b, _ := base64.StdEncoding.DecodeString(match[1])
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(b))
	if cfg.Width != 16 || cfg.Height != 32 {
		t.Fatal("historical rotation not preserved")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationPhotoContentRequiresWorkOwnershipAndScope(t *testing.T) {
	s, mock := mockCreations(t)
	mock.ExpectQuery(`SELECT .* FROM "creations" WHERE id = \$1 AND user_id = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "photo_ids"}).AddRow("work", "[1]"))
	if _, _, err := s.PhotoContent(context.Background(), 7, "work", 2); err == nil {
		t.Fatal("out-of-scope photo exposed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreationSnapshotCannotExpandAuthorizedPhotoScope(t *testing.T) {
	state := map[string]interface{}{"photo_ids": []int64{999}, "photo_edits": map[string]int{"999": 270}, "photo_revision": 999}
	c := model.Creation{PhotoIDs: "[1,2]", State: `{"photo_edits":{"1":90},"photo_revision":3}`}
	sealPhotoSnapshot(state, c, []int64{1})
	if jsonText(state["photo_ids"]) != "[1,2]" || photoRotations(state)["1"] != 90 || photoRotations(state)["999"] != 0 || photoRevision(state) != 3 {
		t.Fatal("agent supplied scope or transforms escaped work authorization")
	}
}
