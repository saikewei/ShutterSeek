//go:build integration

package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
