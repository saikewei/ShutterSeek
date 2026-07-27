package extractor

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"shutterseek/tools/importer/internal/models"
)

// Extractor wraps exiftool subprocess calls.
type Extractor struct {
	exiftoolPath string
}

func New() *Extractor {
	return &Extractor{
		exiftoolPath: "exiftool",
	}
}

// ExtractBatch runs exiftool -json on a batch of file paths.
// Returns metadata for successfully read files (order may differ from input).
func (e *Extractor) ExtractBatch(paths []string) ([]*models.PhotoMetadata, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	args := append([]string{"-json"}, paths...)
	cmd := exec.Command(e.exiftoolPath, args...)
	output, err := cmd.Output()
	if err != nil {
		// exiftool returns exit code 1 if some files failed, but valid output still exists
		if len(output) == 0 {
			return nil, fmt.Errorf("exiftool: %w (stderr)", err)
		}
	}

	var results []*models.PhotoMetadata
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("parse exiftool json: %w", err)
	}

	return results, nil
}

// IsPhotoFormat checks if the file extension is a supported image format.
func IsPhotoFormat(ext string) bool {
	switch strings.ToLower(ext) {
	case ".arw", ".nef", ".rw2", ".dng", ".cr2", ".cr3",
		".orf", ".raf", ".pef", ".sr2", ".srf",
		".jpg", ".jpeg", ".png", ".tif", ".tiff",
		".heic", ".heif", ".hif":
		return true
	}
	return false
}

// IsVideoFormat checks if the file extension is a video format.
func IsVideoFormat(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".mov", ".avi", ".m4v", ".mkv", ".wmv":
		return true
	}
	return false
}
