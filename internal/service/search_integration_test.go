//go:build integration

package service

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSearchDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testDSN()
	if dsn == "" {
		t.Skip("database env vars not set (SHUTTERSEEK_DB_USER etc.)")
	}
	// These regressions only read existing fixtures from the shared database.
	dsn += "&default_transaction_read_only=on&statement_timeout=30000"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	closeTestDB(t, db)
	var readOnly string
	if err := db.Raw("SHOW transaction_read_only").Scan(&readOnly).Error; err != nil || readOnly != "on" {
		t.Fatalf("expected a read-only connection, got %q: %v", readOnly, err)
	}
	return db
}

func TestSearch_FilteredResultsFillLimit(t *testing.T) {
	db := setupSearchDB(t)
	var publicCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM photo_embeddings pe
		JOIN photos p ON p.id = pe.photo_id
		WHERE p.id IN (SELECT ap.photo_id FROM album_photos ap
			JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)`).Scan(&publicCount).Error; err != nil {
		t.Fatal(err)
	}
	var smallestAlbum struct {
		ID    int64
		Count int64
	}
	if err := db.Raw(`SELECT ap.album_id AS id, COUNT(*) AS count
		FROM album_photos ap JOIN photo_embeddings pe ON pe.photo_id = ap.photo_id
		JOIN photos p ON p.id = pe.photo_id
		GROUP BY ap.album_id ORDER BY count, ap.album_id LIMIT 1`).Scan(&smallestAlbum).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(db, &stubEmbedder{vec: vec1024()}, 200)
	tests := []struct {
		name    string
		role    string
		albumID int64
		limit   int
		count   int64
	}{
		{"guest_small_limit", "guest", 0, 5, publicCount},
		{"guest_handler_limit", "guest", 0, 200, publicCount},
		{"small_album", "admin", smallestAlbum.ID, 200, smallestAlbum.Count},
		{"missing_album", "admin", 9223372036854775807, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name != "missing_album" && tt.count == 0 {
				t.Skip("no existing visible embeddings")
			}
			items, err := svc.Search(context.Background(), "filtered regression", tt.role, tt.limit, tt.albumID)
			if err != nil {
				t.Fatal(err)
			}
			want := min(int64(tt.limit), tt.count)
			if int64(len(items)) != want {
				t.Fatalf("returned %d results, want %d from %d eligible embeddings", len(items), want, tt.count)
			}
			seen := make(map[int64]bool)
			for i, item := range items {
				if seen[item.ID] {
					t.Fatalf("duplicate photo %d", item.ID)
				}
				seen[item.ID] = true
				if i > 0 && items[i-1].Score < item.Score {
					t.Fatal("results are not ordered by descending similarity")
				}
				var allowed int64
				query := "SELECT COUNT(*) FROM photos p WHERE p.id = ?"
				args := []any{item.ID}
				if tt.albumID > 0 {
					query += " AND p.id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)"
					args = append(args, tt.albumID)
				}
				if tt.role == "guest" {
					query += ` AND p.id IN (SELECT ap.photo_id FROM album_photos ap
						JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)`
				}
				if err := db.Raw(query, args...).Scan(&allowed).Error; err != nil || allowed != 1 {
					t.Fatalf("photo %d is outside the visible scope: %v", item.ID, err)
				}
			}
		})
	}
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
