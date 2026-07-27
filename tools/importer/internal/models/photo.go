package models

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PhotoMetadata holds the exiftool JSON output for one file.
type PhotoMetadata struct {
	SourceFile string `json:"SourceFile"`
	FileName   string `json:"FileName"`
	FileSize   string `json:"FileSize"`
	MIMEType   string `json:"MIMEType"`

	ImageWidth  int `json:"ImageWidth"`
	ImageHeight int `json:"ImageHeight"`

	DateTimeOriginal string `json:"DateTimeOriginal"`
	CreateDate       string `json:"CreateDate"`

	Make  string `json:"Make"`
	Model string `json:"Model"`

	LensModel string `json:"LensModel"`
	LensInfo  string `json:"LensInfo"`

	FNumber      float64      `json:"FNumber"`
	ISO          int          `json:"ISO"`
	FocalLength  string       `json:"FocalLength"`
	ExposureTime interface{}  `json:"ExposureTime"` // can be string or float64

	FileModifyDate string `json:"FileModifyDate"`

	// For video files:
	Duration string `json:"Duration"`
}

// DBPhoto is the database row representation.
type DBPhoto struct {
	FilePath    string
	FileHash    string
	FileSize    *int64
	Width       *int
	Height      *int
	TakenAt     *time.Time
	CameraMake  *string
	CameraModel *string
	LensModel   *string
	FocalLength *float64
	Aperture    *float64
	ISO         *int
	Status      int
}

// ToDBPhoto converts PhotoMetadata to DBPhoto, applying field mappings.
func (m *PhotoMetadata) ToDBPhoto(photosDir string) *DBPhoto {
	p := &DBPhoto{
		FilePath:   strings.TrimPrefix(m.SourceFile, photosDir+"/"),
		Status:     1,
	}

	// File size: parse "40 MB" or "1481 kB" format
	if size := parseFileSize(m.FileSize); size > 0 {
		p.FileSize = &size
	}

	// Dimensions
	if m.ImageWidth > 0 {
		w := m.ImageWidth
		p.Width = &w
	}
	if m.ImageHeight > 0 {
		h := m.ImageHeight
		p.Height = &h
	}

	// Date taken: prefer DateTimeOriginal, fallback to CreateDate
	dateStr := m.DateTimeOriginal
	if dateStr == "" {
		dateStr = m.CreateDate
	}
	if t := parseExifDate(dateStr); t != nil {
		p.TakenAt = t
	}

	// Camera
	if m.Make != "" {
		v := strings.TrimSpace(m.Make)
		p.CameraMake = &v
	}
	if m.Model != "" {
		v := strings.TrimSpace(m.Model)
		p.CameraModel = &v
	}

	// Lens
	lens := m.LensModel
	if lens == "" {
		lens = m.LensInfo
	}
	if lens != "" {
		v := strings.TrimSpace(lens)
		p.LensModel = &v
	}

	// Focal length: parse "35.0 mm" → 35.0
	if fl := parseFocalLength(m.FocalLength); fl != nil {
		p.FocalLength = fl
	}

	// Aperture
	if m.FNumber > 0 {
		v := m.FNumber
		p.Aperture = &v
	}

	// ISO
	if m.ISO > 0 {
		v := m.ISO
		p.ISO = &v
	}

	// File hash: SHA256 of metadata for fast dedup (no file read needed)
	p.FileHash = computeFileHash(m)

	return p
}

// --- helpers ---

func parseFileSize(s string) int64 {
	if s == "" {
		return 0
	}
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(parts[1])
	switch unit {
	case "KB", "KIB":
		return int64(val * 1024)
	case "MB", "MIB":
		return int64(val * 1024 * 1024)
	case "GB", "GIB":
		return int64(val * 1024 * 1024 * 1024)
	case "B", "BYTES":
		return int64(val)
	default:
		return int64(val)
	}
}

func parseFocalLength(s string) *float64 {
	if s == "" {
		return nil
	}
	// "35.0 mm" → 35.0
	s = strings.TrimSuffix(strings.TrimSpace(s), " mm")
	s = strings.TrimSuffix(s, "mm")
	s = strings.TrimSpace(s)
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return &val
	}
	return nil
}

func parseExifDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	formats := []string{
		"2006:01:02 15:04:05",
		"2006-01-02 15:04:05",
		"2006:01:02 15:04:05-07:00",
		"2006:01:02 15:04:05+07:00",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}

func computeFileHash(m *PhotoMetadata) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d|%d|%.1f|%d",
		m.SourceFile,
		m.FileName,
		m.FileSize,
		m.DateTimeOriginal,
		m.CreateDate,
		m.Make,
		m.Model,
		m.ImageWidth,
		m.ImageHeight,
		m.FNumber,
		m.ISO,
	)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}

// FileExtension returns the lowercase file extension.
func FileExtension(path string) string {
	ext := filepath.Ext(path)
	return strings.ToLower(ext)
}
