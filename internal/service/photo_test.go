//go:build integration

package service

import (
	"context"
	"testing"

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
