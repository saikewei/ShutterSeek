package service

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/tiff"
)

// OriginalService serves original photo files, handling RAW/TIFF extraction.
type OriginalService struct {
	PhotosDir   string
	PreviewDir  string // cached extracted previews
}

func NewOriginalService(photosDir, previewDir string) *OriginalService {
	os.MkdirAll(previewDir, 0755)
	return &OriginalService{PhotosDir: photosDir, PreviewDir: previewDir}
}

// ServeOriginal writes the original photo as JPEG to w.
// For JPG/PNG: reads the file directly.
// For RAW: extracts the embedded JPEG preview.
// For TIFF: decodes via Go and re-encodes as JPEG.
func (s *OriginalService) ServeOriginal(w io.Writer, filePath string) error {
	fullPath := filepath.Join(s.PhotosDir, filePath)
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".heic", ".heif", ".hif":
		return s.serveDirect(w, fullPath)
	case ".tif", ".tiff":
		return s.serveTIFF(w, fullPath)
	default: // RAW: .arw, .nef, .rw2, .dng, .cr2, etc.
		return s.serveRAW(w, fullPath)
	}
}

// serveDirect copies a JPEG/PNG file directly.
func (s *OriginalService) serveDirect(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// serveTIFF decodes a TIFF and re-encodes as JPEG.
func (s *OriginalService) serveTIFF(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open tiff: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode tiff: %w", err)
	}

	if err := jpeg.Encode(w, img, &jpeg.Options{Quality: 92}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
	}
	return nil
}

// serveRAW extracts the embedded JPEG preview from a RAW file.
func (s *OriginalService) serveRAW(w io.Writer, path string) error {
	// Check cache first
	cacheKey := s.PreviewDir + "/" + filepath.Base(path) + ".jpg"
	if data, err := os.ReadFile(cacheKey); err == nil {
		_, err = w.Write(data)
		return err
	}

	// Use Go's standard library to read RAW as a TIFF-like container.
	// Most RAW formats embed a full-resolution JPEG preview accessible via Go's
	// image.DecodeConfig + reading the MakerNote offset.
	//
	// For now, try image.Decode which works for DNG files
	// For other RAW, fall back to reading the embedded JPEG preview bytes.
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open raw: %w", err)
	}
	defer f.Close()

	// Try to decode directly (works for DNG)
	if img, _, err := image.Decode(f); err == nil {
		// Cache and serve
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err == nil {
			os.WriteFile(cacheKey, buf.Bytes(), 0644)
			_, _ = w.Write(buf.Bytes())
			return nil
		}
	}

	// RAW preview extraction via Go native binary search.
	// Most RAW files have a JPEG preview embedded as a byte sequence
	// that starts with 0xFF 0xD8 and ends with 0xFF 0xD9.
	f.Seek(0, 0)
	rawData, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read raw: %w", err)
	}

	jpegData := extractJPEG(rawData)
	if jpegData == nil {
		return fmt.Errorf("no embedded jpeg found in %s", filepath.Base(path))
	}

	os.WriteFile(cacheKey, jpegData, 0644)
	_, err = w.Write(jpegData)
	return err
}

// extractJPEG finds the largest JPEG segment in binary data.
func extractJPEG(data []byte) []byte {
	const (
		soi = 0xFFD8 // Start of Image
		eoi = 0xFFD9 // End of Image
	)

	var best []byte
	pos := 0

	for pos < len(data)-1 {
		if data[pos] == 0xFF && data[pos+1] == 0xD8 {
			// Found SOI — look for EOI
			end := pos + 2
			for end < len(data)-1 {
				if data[end] == 0xFF && data[end+1] == 0xD9 {
					candidate := data[pos : end+2]
					if len(candidate) > len(best) {
						best = candidate
					}
					pos = end + 2
					break
				}
				end++
			}
			if end >= len(data)-1 {
				break
			}
		}
		pos++
	}
	return best
}
