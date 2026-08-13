package service

import (
	"errors"
	"strings"
	"testing"
)

func TestParseVectorValid(t *testing.T) {
	vals := make([]string, 1024)
	for i := range vals {
		vals[i] = "0.03125" // 1/32 → 1024 个的 L2 范数恰为 1
	}
	v, err := ParseVector("[" + strings.Join(vals, ",") + "]")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(v) != 1024 {
		t.Fatalf("len=%d", len(v))
	}
}

func TestParseVectorRejects(t *testing.T) {
	bad := []string{
		"[]",
		"[1,2,3]",
		"not json",
	}
	for _, s := range bad {
		if _, err := ParseVector(s); !errors.Is(err, ErrInvalidVector) {
			t.Errorf("input %q: err=%v", s, err)
		}
	}
}

func TestParseVectorNormBound(t *testing.T) {
	// 1024 个 2.0 → 范数远超 1.1，应拒绝
	v := make([]string, 1024)
	for i := range v {
		v[i] = "2.0"
	}
	if _, err := ParseVector("[" + strings.Join(v, ",") + "]"); !errors.Is(err, ErrInvalidVector) {
		t.Fatal("expected norm rejection")
	}
}

func TestParseVectorRejectsNaN(t *testing.T) {
	vals := make([]string, 1024)
	for i := range vals {
		vals[i] = "0.03125"
	}
	vals[0] = "NaN"
	if _, err := ParseVector("[" + strings.Join(vals, ",") + "]"); !errors.Is(err, ErrInvalidVector) {
		t.Fatal("expected NaN rejection")
	}
}
