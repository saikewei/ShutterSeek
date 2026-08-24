//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupPhotoSvc(t *testing.T) *PhotoService {
	t.Helper()
	db, err := gorm.Open(postgres.Open("postgres://photo_user:PhotoHyc65319436@postgres-main:5432/photo_search?sslmode=disable"), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	return NewPhotoService(db, nil, NewAlbumService(db))
}

func TestPhotoDatesAdminAndGuest(t *testing.T) {
	s := setupPhotoSvc(t)
	admin, err := s.PhotoDates(context.Background(), "admin", 0)
	if err != nil || len(admin) == 0 {
		t.Fatalf("admin dates: %v len=%d", err, len(admin))
	}
	guest, err := s.PhotoDates(context.Background(), "guest", 0)
	if err != nil {
		t.Fatalf("guest dates: %v", err)
	}
	if len(guest) > len(admin) {
		t.Fatalf("guest dates (%d) > admin dates (%d)", len(guest), len(admin))
	}
}

func TestListPhotosFirstPageAndCursor(t *testing.T) {
	s := setupPhotoSvc(t)
	first, err := s.ListPhotos(context.Background(), PhotoListParams{Limit: 45, Role: "admin"})
	if err != nil || len(first.Photos) == 0 {
		t.Fatalf("first page: %v len=%d", err, len(first.Photos))
	}
	if first.Total < int64(len(first.Photos)) {
		t.Fatalf("total %d < page %d", first.Total, len(first.Photos))
	}
	if first.HasMore && first.NextCursor == "" {
		t.Fatal("expected next cursor when has more")
	}
	if first.HasMore {
		second, err := s.ListPhotos(context.Background(), PhotoListParams{
			Limit: 45, Role: "admin", Cursor: first.NextCursor,
		})
		if err != nil {
			t.Fatalf("second page: %v", err)
		}
		if len(second.Photos) == 0 || second.Photos[0].ID == first.Photos[0].ID {
			t.Fatal("second page must continue after first")
		}
	}
}

func TestListPhotosGuestScope(t *testing.T) {
	s := setupPhotoSvc(t)
	admin, _ := s.ListPhotos(context.Background(), PhotoListParams{Limit: 45, Role: "admin"})
	guest, err := s.ListPhotos(context.Background(), PhotoListParams{Limit: 45, Role: "guest"})
	if err != nil {
		t.Fatalf("guest: %v", err)
	}
	if len(guest.Photos) > len(admin.Photos) {
		t.Fatalf("guest (%d) > admin (%d)", len(guest.Photos), len(admin.Photos))
	}
	if guest.Total > admin.Total {
		t.Fatalf("guest total %d > admin total %d", guest.Total, admin.Total)
	}
}

func TestListPhotosMonthJumpHead(t *testing.T) {
	s := setupPhotoSvc(t)
	res, err := s.ListPhotos(context.Background(), PhotoListParams{
		Limit: 45, Role: "admin", Month: time.Now().In(cstZone).Format("2006-01"),
	})
	if err != nil {
		t.Fatalf("month jump: %v", err)
	}
	if len(res.HeadPhotos) > 15 {
		t.Fatalf("head preload capped at 15, got %d", len(res.HeadPhotos))
	}
	if res.Total < 0 || len(res.Photos) > 45 {
		t.Fatalf("unexpected shape: total=%d page=%d", res.Total, len(res.Photos))
	}
}

func TestGetPhotoAndPublicAlbumGuard(t *testing.T) {
	s := setupPhotoSvc(t)
	res, err := s.ListPhotos(context.Background(), PhotoListParams{Limit: 1, Role: "admin"})
	if err != nil || len(res.Photos) == 0 {
		t.Fatalf("seed photo: %v", err)
	}
	p, err := s.GetPhoto(context.Background(), res.Photos[0].ID)
	if err != nil || p.ID == 0 {
		t.Fatalf("GetPhoto: %v", err)
	}
	if _, err := s.GetPhoto(context.Background(), -1); err == nil {
		t.Fatal("expected error for missing photo")
	}
	ok, err := s.PhotoInPublicAlbum(context.Background(), res.Photos[0].ID)
	if err != nil {
		t.Fatalf("PhotoInPublicAlbum: %v", err)
	}
	if !ok {
		g, _ := s.ListPhotos(context.Background(), PhotoListParams{Limit: 1, Role: "guest"})
		if len(g.Photos) == 0 {
			t.Skip("no public-album photos in library")
		}
		ok, err = s.PhotoInPublicAlbum(context.Background(), g.Photos[0].ID)
		if err != nil || !ok {
			t.Fatalf("guest-visible photo must be in public album: %v", err)
		}
	}
}
