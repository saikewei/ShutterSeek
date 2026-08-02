//go:build integration

package service

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupSearchDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testDSN()
	if dsn == "" {
		t.Skip("database env vars not set (SHUTTERSEEK_DB_USER etc.)")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	return db
}

func parseVectorText(t *testing.T, s string) []float32 {
	t.Helper()
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			t.Fatalf("parse %q: %v", p, err)
		}
		out = append(out, float32(f))
	}
	return out
}

func TestSearch_Top1MatchesExactEmbedding(t *testing.T) {
	db := setupSearchDB(t)
	var photoID int64
	var vecText string
	if err := db.Raw("SELECT photo_id, embedding::text FROM photo_embeddings LIMIT 1").
		Row().Scan(&photoID, &vecText); err != nil {
		t.Fatal(err)
	}
	vec := parseVectorText(t, vecText)

	svc := NewSearchService(db, &stubEmbedder{vec: vec}, 200)
	items, err := svc.Search(context.Background(), "exact match", "admin", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("no results")
	}
	if items[0].ID != photoID {
		t.Fatalf("top1 = %d, want %d", items[0].ID, photoID)
	}
	if items[0].Score < 0.999 {
		t.Fatalf("score = %f, want ~1.0", items[0].Score)
	}
}

func TestSearch_AlbumScoped(t *testing.T) {
	db := setupSearchDB(t)
	var albumID, photoID int64
	if err := db.Raw(`SELECT ap.album_id, ap.photo_id
		FROM album_photos ap
		JOIN photo_embeddings pe ON pe.photo_id = ap.photo_id
		LIMIT 1`).Row().Scan(&albumID, &photoID); err != nil {
		t.Fatal(err)
	}
	var vecText string
	if err := db.Raw("SELECT embedding::text FROM photo_embeddings WHERE photo_id = ?", photoID).
		Row().Scan(&vecText); err != nil {
		t.Fatal(err)
	}
	vec := parseVectorText(t, vecText)

	svc := NewSearchService(db, &stubEmbedder{vec: vec}, 200)
	items, err := svc.Search(context.Background(), "album", "admin", 50, albumID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].ID != photoID {
		t.Fatalf("album-scoped top result wrong: %+v", items)
	}
	for _, it := range items {
		var cnt int64
		db.Raw("SELECT COUNT(*) FROM album_photos WHERE album_id = ? AND photo_id = ?",
			albumID, it.ID).Scan(&cnt)
		if cnt == 0 {
			t.Fatalf("photo %d not in album %d", it.ID, albumID)
		}
	}
}

func TestSearch_GuestOnlyPublicAlbums(t *testing.T) {
	db := setupSearchDB(t)
	var photoID int64
	var vecText string
	if err := db.Raw(`SELECT pe.photo_id, pe.embedding::text
		FROM photo_embeddings pe
		JOIN album_photos ap ON ap.photo_id = pe.photo_id
		JOIN albums a ON a.id = ap.album_id AND a.is_public = true
		LIMIT 1`).Row().Scan(&photoID, &vecText); err != nil {
		t.Fatal(err)
	}
	vec := parseVectorText(t, vecText)

	svc := NewSearchService(db, &stubEmbedder{vec: vec}, 200)
	items, err := svc.Search(context.Background(), "guest", "guest", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].ID != photoID {
		t.Fatalf("guest top result wrong: %+v", items)
	}
	for _, it := range items {
		var cnt int64
		db.Raw(`SELECT COUNT(*) FROM album_photos ap
			JOIN albums a ON a.id = ap.album_id
			WHERE ap.photo_id = ? AND a.is_public = true`, it.ID).Scan(&cnt)
		if cnt == 0 {
			t.Fatalf("guest saw non-public photo %d", it.ID)
		}
	}
}
