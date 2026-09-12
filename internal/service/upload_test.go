package service

import "testing"

func TestErrDuplicate(t *testing.T) {
	e := ErrDuplicate{ExistingID: 42}
	if e.Error() != "duplicate photo id=42" {
		t.Fatalf("err: %s", e.Error())
	}
}

func TestUploadAbsPath(t *testing.T) {
	if got := uploadAbsPath("/photos_uploads", "uploads/2026/08/a.jpg"); got != "/photos_uploads/2026/08/a.jpg" {
		t.Fatalf("got %q", got)
	}
	if got := uploadAbsPath("/photos_uploads", "2026/08/a.jpg"); got != "/photos_uploads/2026/08/a.jpg" {
		t.Fatalf("no-prefix got %q", got)
	}
}
