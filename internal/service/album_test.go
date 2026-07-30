//go:build integration

package service

import (
	"errors"
	"sort"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupAlbumSvc(t *testing.T) *AlbumService {
	t.Helper()
	dsn := "postgres://photo_user:PhotoHyc65319436@postgres-main:5432/photo_search?sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	// Clean up any leftover test albums
	db.Exec("DELETE FROM album_photos WHERE album_id IN (SELECT id FROM albums WHERE title LIKE 'TEST_%')")
	db.Exec("DELETE FROM albums WHERE title LIKE 'TEST_%'")
	return NewAlbumService(db)
}

// ═══════════════════════════════════════════════════════
// CreateAlbum
// ═══════════════════════════════════════════════════════

func TestCreateAlbum_Success(t *testing.T) {
	svc := setupAlbumSvc(t)

	item, err := svc.CreateAlbum("TEST_Create_OK", "desc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("expected ID > 0")
	}
	if item.Title != "TEST_Create_OK" {
		t.Fatalf("title mismatch: %s", item.Title)
	}
	if item.PhotoCount != 0 {
		t.Fatalf("new album should have 0 photos, got %d", item.PhotoCount)
	}
	// Cleanup
	svc.DeleteAlbum(item.ID)
}

func TestCreateAlbum_DuplicateTitle(t *testing.T) {
	svc := setupAlbumSvc(t)

	a1, err := svc.CreateAlbum("TEST_Dup", "")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	defer svc.DeleteAlbum(a1.ID)

	a2, err := svc.CreateAlbum("TEST_Dup", "")
	if err != nil {
		// UNIQUE constraint exists — this is fine, but clean up the first
		svc.DeleteAlbum(a1.ID)
		return
	}
	// If no constraint, both created, clean up both
	defer svc.DeleteAlbum(a2.ID)
	t.Log("duplicate title allowed (no unique constraint)")
}

// ═══════════════════════════════════════════════════════
// GetAlbum
// ═══════════════════════════════════════════════════════

func TestGetAlbum_Found(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Get", "")
	defer svc.DeleteAlbum(created.ID)

	item, err := svc.GetAlbum(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item.Title != "TEST_Get" {
		t.Fatalf("title mismatch: %s", item.Title)
	}
}

func TestGetAlbum_NotFound(t *testing.T) {
	svc := setupAlbumSvc(t)
	_, err := svc.GetAlbum(99999999)
	if err == nil {
		t.Fatal("expected error for non-existent album")
	}
}

// ═══════════════════════════════════════════════════════
// ListAlbums
// ═══════════════════════════════════════════════════════

func TestListAlbums_ReturnsAll(t *testing.T) {
	svc := setupAlbumSvc(t)
	items, err := svc.ListAlbums()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least some albums")
	}
	// Verify all have IDs and titles
	for _, it := range items {
		if it.ID == 0 || it.Title == "" {
			t.Fatalf("malformed item: %+v", it)
		}
	}
}

func TestListAlbums_IncludesPhotoCount(t *testing.T) {
	svc := setupAlbumSvc(t)
	items, _ := svc.ListAlbums()
	for _, it := range items {
		if it.PhotoCount < 0 {
			t.Fatalf("negative photo count for %s", it.Title)
		}
	}
}

// ═══════════════════════════════════════════════════════
// UpdateAlbum
// ═══════════════════════════════════════════════════════

func TestUpdateAlbum_Rename(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Update_Rename", "")
	defer svc.DeleteAlbum(created.ID)

	newTitle := "TEST_Update_Renamed"
	updated, err := svc.UpdateAlbum(created.ID, &newTitle, nil, nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != newTitle {
		t.Fatalf("title not updated: %s", updated.Title)
	}
}

func TestUpdateAlbum_SetCover(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Update_Cover", "")
	defer svc.DeleteAlbum(created.ID)

	// First add a photo to the album so cover can reference it
	var firstPhotoID int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 1").Scan(&firstPhotoID)
	if firstPhotoID == 0 {
		t.Skip("no photos in DB")
	}
	svc.BatchAddPhotos(created.ID, []int64{firstPhotoID})

	updated, err := svc.UpdateAlbum(created.ID, nil, nil, &firstPhotoID)
	if err != nil {
		t.Fatalf("set cover: %v", err)
	}
	if updated.CoverURL == "" {
		t.Fatal("cover URL should be set")
	}
}

func TestUpdateAlbum_ClearCover(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Update_ClearCover", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos ORDER BY id LIMIT 2").Scan(&photoIDs)
	if len(photoIDs) < 2 {
		t.Skip("not enough photos")
	}
	firstPhotoID := photoIDs[0]
	secondPhotoID := photoIDs[1]
	svc.BatchAddPhotos(created.ID, []int64{firstPhotoID, secondPhotoID})

	// Set cover to second photo
	svc.UpdateAlbum(created.ID, nil, nil, &secondPhotoID)

	// Clear it: should fall back to first photo (auto-pick by sort_order)
	neg := int64(-1)
	updated, err := svc.UpdateAlbum(created.ID, nil, nil, &neg)
	if err != nil {
		t.Fatalf("clear cover: %v", err)
	}
	if updated.CoverURL == "" {
		t.Fatal("cover URL should fall back to first photo, not empty")
	}
	t.Logf("cleared cover, auto-picked: %s", updated.CoverURL)
}

func TestUpdateAlbum_CoverPhotoNotInAlbum(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Update_BadCover", "")
	defer svc.DeleteAlbum(created.ID)

	// Try to set cover to a photo NOT in this album
	badID := int64(1)
	_, err := svc.UpdateAlbum(created.ID, nil, nil, &badID)
	if !errors.Is(err, ErrPhotoNotInAlbum) {
		t.Fatalf("expected ErrPhotoNotInAlbum, got: %v", err)
	}
}

func TestUpdateAlbum_NotFound(t *testing.T) {
	svc := setupAlbumSvc(t)
	title := "x"
	_, err := svc.UpdateAlbum(99999999, &title, nil, nil)
	if !errors.Is(err, ErrAlbumNotFound) {
		t.Fatalf("expected ErrAlbumNotFound, got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════
// DeleteAlbum
// ═══════════════════════════════════════════════════════

func TestDeleteAlbum_CascadesPhotos(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Del_Cascade", "")
	defer func() { svc.DeleteAlbum(created.ID) }()

	// Add photos
	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 3").Scan(&photoIDs)
	svc.BatchAddPhotos(created.ID, photoIDs)

	// Delete
	if err := svc.DeleteAlbum(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify associations are gone
	var count int64
	svc.DB.Raw("SELECT COUNT(*) FROM album_photos WHERE album_id = ?", created.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("cascade failed: %d associations remain", count)
	}
}

func TestDeleteAlbum_NotFound(t *testing.T) {
	svc := setupAlbumSvc(t)
	err := svc.DeleteAlbum(99999999)
	// Delete with a non-existent ID is a no-op in GORM, no error expected.
	// We just verify the function doesn't panic.
	_ = err
}

// ═══════════════════════════════════════════════════════
// RemoveAlbumPhoto
// ═══════════════════════════════════════════════════════

func TestRemoveAlbumPhoto_Success(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Remove_OK", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 2").Scan(&photoIDs)
	svc.BatchAddPhotos(created.ID, photoIDs)

	if err := svc.RemoveAlbumPhoto(created.ID, photoIDs[0]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Verify removed
	var count int64
	svc.DB.Raw("SELECT COUNT(*) FROM album_photos WHERE album_id = ? AND photo_id = ?",
		created.ID, photoIDs[0]).Scan(&count)
	if count != 0 {
		t.Fatal("photo not removed")
	}
}

func TestRemoveAlbumPhoto_NotFound(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Remove_NF", "")
	defer svc.DeleteAlbum(created.ID)

	err := svc.RemoveAlbumPhoto(created.ID, 99999999)
	if !errors.Is(err, ErrPhotoNotInAlbum) {
		t.Fatalf("expected ErrPhotoNotInAlbum, got: %v", err)
	}
}

func TestRemoveAlbumPhoto_ClearsCover(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Remove_Cover", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 1").Scan(&photoIDs)
	svc.BatchAddPhotos(created.ID, photoIDs)
	svc.UpdateAlbum(created.ID, nil, nil, &photoIDs[0])

	// Remove the cover photo
	if err := svc.RemoveAlbumPhoto(created.ID, photoIDs[0]); err != nil {
		t.Fatalf("remove cover: %v", err)
	}

	// Verify cover is cleared
	updated, _ := svc.GetAlbum(created.ID)
	if updated.CoverURL != "" {
		t.Fatal("cover URL should be cleared when cover photo is removed")
	}
}

// ═══════════════════════════════════════════════════════
// BatchAddPhotos
// ═══════════════════════════════════════════════════════

func TestBatchAddPhotos_Success(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Batch_OK", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 5").Scan(&photoIDs)
	if len(photoIDs) < 5 {
		t.Skip("not enough photos")
	}

	result, err := svc.BatchAddPhotos(created.ID, photoIDs)
	if err != nil {
		t.Fatalf("batch add: %v", err)
	}
	if result.Added != 5 {
		t.Fatalf("expected 5 added, got %d", result.Added)
	}
	if result.Skipped != 0 {
		t.Fatalf("expected 0 skipped, got %d", result.Skipped)
	}
}

func TestBatchAddPhotos_DuplicatesSkipped(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Batch_Dup", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 3").Scan(&photoIDs)

	// First batch
	svc.BatchAddPhotos(created.ID, photoIDs)

	// Second batch with same + new
	mixed := append(photoIDs, photoIDs[0])
	result, err := svc.BatchAddPhotos(created.ID, mixed)
	if err != nil {
		t.Fatalf("batch add: %v", err)
	}
	if result.Added != 0 {
		t.Fatalf("expected 0 added (all exist), got %d", result.Added)
	}
	if result.Skipped < 3 {
		t.Fatalf("expected at least 3 skipped, got %d", result.Skipped)
	}
}

func TestBatchAddPhotos_AlbumNotFound(t *testing.T) {
	svc := setupAlbumSvc(t)
	_, err := svc.BatchAddPhotos(99999999, []int64{1})
	if !errors.Is(err, ErrAlbumNotFound) {
		t.Fatalf("expected ErrAlbumNotFound, got: %v", err)
	}
}

func TestBatchAddPhotos_EmptyList(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Batch_Empty", "")
	defer svc.DeleteAlbum(created.ID)

	result, err := svc.BatchAddPhotos(created.ID, nil)
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if result.Added != 0 || result.Skipped != 0 {
		t.Fatalf("expected 0/0, got %d/%d", result.Added, result.Skipped)
	}
}

func TestBatchAddPhotos_NonexistentPhoto(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Batch_BadPhoto", "")
	defer svc.DeleteAlbum(created.ID)

	_, err := svc.BatchAddPhotos(created.ID, []int64{99999999})
	if err == nil {
		t.Fatal("expected FK error for non-existent photo")
	}
}

// ═══════════════════════════════════════════════════════
// GetPhotoAlbumIDs
// ═══════════════════════════════════════════════════════

func TestGetPhotoAlbumIDs_Success(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_IDs_OK", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 3").Scan(&photoIDs)
	svc.BatchAddPhotos(created.ID, photoIDs)

	result, err := svc.GetPhotoAlbumIDs(photoIDs)
	if err != nil {
		t.Fatalf("get album IDs: %v", err)
	}
	for _, pid := range photoIDs {
		ids := result[pid]
		found := false
		for _, aid := range ids {
			if aid == created.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("photo %d should be in album %d, got %v", pid, created.ID, ids)
		}
	}
}

func TestGetPhotoAlbumIDs_EmptyInput(t *testing.T) {
	svc := setupAlbumSvc(t)
	result, err := svc.GetPhotoAlbumIDs(nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for empty input")
	}
}

func TestGetPhotoAlbumIDs_NoAlbums(t *testing.T) {
	svc := setupAlbumSvc(t)
	// Pick photos that are not in any album
	result, err := svc.GetPhotoAlbumIDs([]int64{99999999})
	if err != nil {
		t.Fatalf("no albums: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

// ═══════════════════════════════════════════════════════
// ListAlbumPhotos (pagination + edge cases)
// ═══════════════════════════════════════════════════════

func TestListAlbumPhotos_WithPhotos(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_ListPhotos", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos LIMIT 10").Scan(&photoIDs)
	svc.BatchAddPhotos(created.ID, photoIDs)

	page, err := svc.ListAlbumPhotos(created.ID, 5, zeroTime(), 0)
	if err != nil {
		t.Fatalf("list photos: %v", err)
	}
	if len(page.Photos) != 5 {
		t.Fatalf("expected 5 photos, got %d", len(page.Photos))
	}
	if !page.HasMore {
		t.Fatal("expected HasMore with 10 photos and limit 5")
	}
	if page.Total != 10 {
		t.Fatalf("expected total 10, got %d", page.Total)
	}
}

func TestListAlbumPhotos_EmptyAlbum(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_ListEmpty", "")
	defer svc.DeleteAlbum(created.ID)

	page, err := svc.ListAlbumPhotos(created.ID, 50, zeroTime(), 0)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(page.Photos) != 0 {
		t.Fatalf("expected 0 photos, got %d", len(page.Photos))
	}
	if page.HasMore {
		t.Fatal("should not have more")
	}
}

func TestListAlbumPhotos_CursorPagination(t *testing.T) {
	svc := setupAlbumSvc(t)
	created, _ := svc.CreateAlbum("TEST_Cursor", "")
	defer svc.DeleteAlbum(created.ID)

	var photoIDs []int64
	svc.DB.Raw("SELECT id FROM photos ORDER BY taken_at DESC LIMIT 10").Scan(&photoIDs)
	sort.Slice(photoIDs, func(i, j int) bool { return photoIDs[i] > photoIDs[j] })
	svc.BatchAddPhotos(created.ID, photoIDs)

	// First page
	p1, _ := svc.ListAlbumPhotos(created.ID, 3, zeroTime(), 0)
	if len(p1.Photos) != 3 {
		t.Fatalf("page 1: expected 3, got %d", len(p1.Photos))
	}

	// Second page using cursor from last item
	last := p1.Photos[2]
	p2, _ := svc.ListAlbumPhotos(created.ID, 3, last.TakenAt, last.ID)
	if len(p2.Photos) == 0 {
		t.Fatal("page 2 should have photos")
	}

	// Verify no overlap
	ids1 := map[int64]bool{}
	for _, p := range p1.Photos {
		ids1[p.ID] = true
	}
	for _, p := range p2.Photos {
		if ids1[p.ID] {
			t.Fatalf("photo %d appears on both pages", p.ID)
		}
	}
}

// ═══════════════════════════════════════════════════════
// helpers
// ═══════════════════════════════════════════════════════

func zeroTime() time.Time { return time.Time{} }
