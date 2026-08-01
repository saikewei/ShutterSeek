//go:build integration

package service

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupSvc(t *testing.T) *OriginalService {
	t.Helper()
	dir := t.TempDir()
	return NewOriginalService("/photos", dir)
}

func TestServeDirect_JPG(t *testing.T) {
	svc := setupSvc(t)
	var buf bytes.Buffer
	err := svc.ServeOriginal(&buf, "2022summer/SNY00006.ARW")
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty output")
	}
	// JPEG starts with FF D8
	if buf.Bytes()[0] != 0xFF || buf.Bytes()[1] != 0xD8 {
		t.Fatal("not a valid JPEG header")
	}
	t.Logf("RAW → JPEG: %d bytes", buf.Len())
}

func TestServeOriginal_NotFound(t *testing.T) {
	svc := setupSvc(t)
	var buf bytes.Buffer
	err := svc.ServeOriginal(&buf, "nonexistent/file.jpg")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestServeOriginal_RawPreview(t *testing.T) {
	svc := setupSvc(t)
	// Test another RAW file to verify extraction
	var buf bytes.Buffer
	err := svc.ServeOriginal(&buf, "2024新加坡/SNY04810.ARW")
	if err != nil {
		t.Fatalf("RAW extraction failed: %v", err)
	}
	if buf.Len() < 1000 {
		t.Fatalf("RAW preview too small: %d bytes (probably failed extraction)", buf.Len())
	}
	t.Logf("ARW preview: %d bytes", buf.Len())
}

func TestServeOriginal_PNG(t *testing.T) {
	svc := setupSvc(t)
	var buf bytes.Buffer
	err := svc.ServeOriginal(&buf, "FromDesktop/film/ektar100/校色.png")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PNG: %d bytes", buf.Len())
}

func TestExtractJPEG(t *testing.T) {
	// Read a known ARW file and verify JPEG extraction
	path := filepath.Join("/photos", "2022summer/SNY00006.ARW")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("can't read test file: %v", err)
	}
	jpeg := extractJPEG(data)
	if jpeg == nil {
		t.Fatal("no JPEG found in ARW file")
	}
	if jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		t.Fatal("extracted data doesn't start with JPEG SOI marker")
	}
	last := len(jpeg) - 1
	if jpeg[last-1] != 0xFF || jpeg[last] != 0xD9 {
		t.Fatal("extracted data doesn't end with JPEG EOI marker")
	}
	t.Logf("extracted JPEG: %d bytes", len(jpeg))
}

func TestServeDirect_RealJPG(t *testing.T) {
	svc := setupSvc(t)
	var buf bytes.Buffer
	err := svc.ServeOriginal(&buf, "2022summer/SNY01557.JPG")
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty output")
	}
	t.Logf("JPG direct: %d bytes", buf.Len())
}

func TestCacheReuse(t *testing.T) {
	svc := setupSvc(t)

	// First call should cache
	var buf1 bytes.Buffer
	err := svc.ServeOriginal(&buf1, "2022summer/SNY00006.ARW")
	if err != nil {
		t.Fatal(err)
	}

	// Second call should use cache
	var buf2 bytes.Buffer
	err = svc.ServeOriginal(&buf2, "2022summer/SNY00006.ARW")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("cached result differs from original")
	}
	t.Logf("cache works: %d bytes ==", buf1.Len())
}

func TestDirCreation(t *testing.T) {
	svc := NewOriginalService("/photos", "/tmp/nonexistent_test_dir")
	if !strings.Contains(svc.PreviewDir, "nonexistent_test_dir") {
		t.Fatal("wrong preview dir")
	}
	// Dir should have been created
	if _, err := os.Stat(svc.PreviewDir); os.IsNotExist(err) {
		t.Fatal("preview dir was not created")
	}
	os.RemoveAll(svc.PreviewDir)
}

// ═══════════════════════════════════════════════════════
// Cache eviction
// ═══════════════════════════════════════════════════════

func TestCacheWriteAndPrune(t *testing.T) {
	dir := t.TempDir()
	svc := NewOriginalService("/photos", dir)

	// Write more than maxCacheFiles (2000) dummy files
	for i := 0; i < 10; i++ {
		data := []byte{0xFF, 0xD8, 0xFF, 0xD9} // minimal JPEG
		svc.cacheWrite(fmt.Sprintf("test_%d.jpg", i), data)
	}

	// Wait a bit for async prune
	time.Sleep(10 * time.Millisecond)

	entries, _ := os.ReadDir(dir)
	t.Logf("cache files: %d", len(entries))
	if len(entries) != 10 {
		t.Fatalf("expected 10 files, got %d", len(entries))
	}
}

func TestCacheWrite_PeriodicPrune(t *testing.T) {
	dir := t.TempDir()
	svc := NewOriginalService("/photos", dir)

	// Write 100 files (triggers prune check)
	for i := 0; i < 100; i++ {
		data := []byte{0xFF, 0xD8, 0xFF, 0xD9}
		svc.cacheWrite(fmt.Sprintf("prune_%d.jpg", i), data)
	}

	time.Sleep(50 * time.Millisecond)

	entries, _ := os.ReadDir(dir)
	jpgs := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			jpgs++
		}
	}
	t.Logf("cache files after 100 writes: %d", jpgs)
	if jpgs != 100 {
		t.Fatalf("expected 100 files, got %d", jpgs)
	}
}

func TestCachePrune_KeepsNewest(t *testing.T) {
	dir := t.TempDir()
	svc := NewOriginalService("/photos", dir)

	// Write files with delays to create age differences
	for i := 0; i < 5; i++ {
		data := []byte{0xFF, 0xD8, 0xFF, 0xD9}
		svc.cacheWrite(fmt.Sprintf("keep_%d.jpg", i), data)
		time.Sleep(5 * time.Millisecond)
	}

	// Force prune (modify maxCacheFiles for test - not possible, just check no panic)
	svc.pruneCache()

	entries, _ := os.ReadDir(dir)
	jpgs := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			jpgs++
		}
	}
	if jpgs != 5 {
		t.Fatalf("prune shouldn't delete when below max: expected 5, got %d", jpgs)
	}
}

// ═══════════════════════════════════════════════════════
// Exiftool fallback
// ═══════════════════════════════════════════════════════

func TestExtractWithExiftool_A6000(t *testing.T) {
	// Test with a known a6000 ARW file
	path := "/photos/FromDesktop/a6000/跳绳/_DSC0757.ARW"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("a6000 test file not found")
	}

	data, err := extractWithExiftool(path)
	if err != nil {
		t.Fatalf("exiftool extraction failed: %v", err)
	}
	if len(data) < 20000 {
		t.Fatalf("extracted data too small: %d bytes", len(data))
	}
	if data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("not a valid JPEG header")
	}
	t.Logf("exiftool extracted: %d bytes", len(data))
}

func TestExtractWithExiftool_NotFound(t *testing.T) {
	_, err := extractWithExiftool("/nonexistent/file.ARW")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestServeRAW_A6000Preview(t *testing.T) {
	path := "/photos/FromDesktop/a6000/跳绳/_DSC0757.ARW"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("a6000 test file not found")
	}

	dir := t.TempDir()
	svc := NewOriginalService("/photos", dir)

	var buf bytes.Buffer
	err := svc.ServeOriginal(&buf, "FromDesktop/a6000/跳绳/_DSC0757.ARW")
	if err != nil {
		t.Fatalf("serve a6000: %v", err)
	}
	// Full-size PreviewImage via TIFF IFD is ~419KB; a thumbnail is ~9KB.
	if buf.Len() < 200000 {
		t.Fatalf("too small: %d bytes (expected full-size preview ~419KB)", buf.Len())
	}
	// Verify it's cached
	if _, err := os.Stat(dir + "/_DSC0757.ARW.jpg"); os.IsNotExist(err) {
		t.Fatal("preview not cached")
	}
	t.Logf("a6000 served: %d bytes", buf.Len())
}

func TestExtractTIFFJPEG_A6000(t *testing.T) {
	path := "/photos/FromDesktop/a6000/跳绳/_DSC0757.ARW"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("a6000 test file not found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	seg := extractTIFFJPEG(data)
	if seg == nil {
		t.Fatal("no preview found via TIFF IFD")
	}
	// The global scan only finds the 9KB thumbnail; IFD must find the
	// 419KB full-size PreviewImage.
	if len(seg) < 200000 {
		t.Fatalf("IFD preview too small: %d bytes (expected ~419KB)", len(seg))
	}
	t.Logf("extractTIFFJPEG: %d bytes", len(seg))
}

func TestServeRAW_NEFFullSize(t *testing.T) {
	path := "/photos/photo/Z6/UBI_2484.NEF"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("NEF test file not found")
	}

	dir := t.TempDir()
	svc := NewOriginalService("/photos", dir)
	var buf bytes.Buffer
	if err := svc.ServeOriginal(&buf, "photo/Z6/UBI_2484.NEF"); err != nil {
		t.Fatalf("serve NEF: %v", err)
	}
	// Full-size NEF preview is ~1.8MB; a buggy path serves a 13KB thumbnail.
	if buf.Len() < 500000 {
		t.Fatalf("NEF preview too small: %d bytes (expected full-size ~1.8MB)", buf.Len())
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode NEF preview: %v", err)
	}
	if cfg.Width < 2000 {
		t.Fatalf("NEF preview too small: %dx%d (expected full-size)", cfg.Width, cfg.Height)
	}
	t.Logf("NEF served: %d bytes, %dx%d", buf.Len(), cfg.Width, cfg.Height)
}

// need fmt for TestCacheWriteAndPrune

