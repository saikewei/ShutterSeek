package service

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "golang.org/x/image/tiff"
)

// OriginalService serves original photo files, handling RAW/TIFF extraction.
type OriginalService struct {
	PhotosDir   string
	PreviewDir  string // cached extracted previews
	mu          sync.Mutex
	pruneCount  int // track writes between prunes
}

const maxCacheFiles = 2000

func NewOriginalService(photosDir, previewDir string) *OriginalService {
	os.MkdirAll(previewDir, 0755)
	return &OriginalService{PhotosDir: photosDir, PreviewDir: previewDir}
}

// cacheWrite writes data to the cache and periodically prunes old files.
func (s *OriginalService) cacheWrite(name string, data []byte) {
	path := filepath.Join(s.PreviewDir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return
	}
	s.mu.Lock()
	s.pruneCount++
	n := s.pruneCount
	s.mu.Unlock()
	if n%100 == 0 {
		go s.pruneCache()
	}
}

func (s *OriginalService) pruneCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.PreviewDir)
	if err != nil || len(entries) <= maxCacheFiles {
		return
	}

	type fi struct {
		name    string
		modTime int64
	}
	var files []fi
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fi{e.Name(), info.ModTime().Unix()})
	}

	// Keep the newest maxCacheFiles, delete the rest
	if len(files) <= maxCacheFiles {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime > files[j].modTime })
	for _, f := range files[maxCacheFiles:] {
		os.Remove(filepath.Join(s.PreviewDir, f.name))
	}
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
// Strategy: largest embedded JPEG first (via binary scan), then Go decode
// (DNG-style), then exiftool. image.Decode must NOT be first — Go's TIFF
// decoder reads a small thumbnail from many RAW containers (e.g. Nikon NEF
// IFD0) and would otherwise serve a 160x120 image instead of the full-size
// preview.
func (s *OriginalService) serveRAW(w io.Writer, path string) error {
	// Check cache first (ignore tiny/broken previews)
	cacheKey := s.PreviewDir + "/" + filepath.Base(path) + ".jpg"
	if data, err := os.ReadFile(cacheKey); err == nil && len(data) >= minPreviewSize {
		_, err = w.Write(data)
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open raw: %w", err)
	}
	defer f.Close()

	rawData, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read raw: %w", err)
	}

	// 1. TIFF IFD — precise preview location (NEF/ARW/CR2/DNG etc.)
	if jpegData := extractTIFFJPEG(rawData); len(jpegData) >= minPreviewSize {
		s.cacheWrite(filepath.Base(path)+".jpg", jpegData)
		_, err = w.Write(jpegData)
		return err
	}

	// 2. Global binary scan (non-TIFF containers or IFD-missed previews)
	if jpegData := extractJPEG(rawData); len(jpegData) >= minPreviewSize {
		s.cacheWrite(filepath.Base(path)+".jpg", jpegData)
		_, err = w.Write(jpegData)
		return err
	}

	// 3. Decode via Go (DNG and other RAW with decodable image data).
	// Guard the output size so a decoded thumbnail is rejected.
	f.Seek(0, 0)
	if img, _, err := image.Decode(f); err == nil {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err == nil && buf.Len() >= minPreviewSize {
			s.cacheWrite(filepath.Base(path)+".jpg", buf.Bytes())
			_, _ = w.Write(buf.Bytes())
			return nil
		}
	}

	// 4. exiftool — last resort only (rare: formats no Go path handles).
	if data, err := extractWithExiftool(path); err == nil {
		s.cacheWrite(filepath.Base(path)+".jpg", data)
		_, err = w.Write(data)
		return err
	}

	return fmt.Errorf("no usable preview in %s", filepath.Base(path))
}

// minPreviewSize is the smallest extracted preview we consider a real
// full-size image (not a thumbnail).
const minPreviewSize = 20000

// extractWithExiftool extracts a JPEG preview using exiftool.
// JpgFromRaw is tried first (full-size in Nikon NEF); among the tags that
// yield a valid large JPEG, the largest is returned.
func extractWithExiftool(path string) ([]byte, error) {
	var best []byte
	for _, tag := range []string{"-JpgFromRaw", "-PreviewImage"} {
		data, err := exec.Command("exiftool", "-b", tag, path).Output()
		if err == nil && len(data) >= minPreviewSize && data[0] == 0xFF && data[1] == 0xD8 {
			if len(data) > len(best) {
				best = data
			}
		}
	}
	if best != nil {
		return best, nil
	}
	return nil, fmt.Errorf("exiftool extraction failed")
}

// extractTIFFJPEG finds the largest embedded JPEG preview by walking the
// TIFF IFD tree and reading each JPEGInterchangeFormat (0x0201) offset.
// RAW formats (NEF/ARW/CR2/DNG) are TIFF containers; this precisely locates
// full-size previews that a global binary scan can miss (e.g. Sony a6000,
// where extractJPEG only finds the 9KB thumbnail but the 419KB PreviewImage
// sits behind a JPEGInterchangeFormat tag). No external process needed.
func extractTIFFJPEG(data []byte) []byte {
	if len(data) < 8 {
		return nil
	}
	var order binary.ByteOrder
	switch {
	case data[0] == 'I' && data[1] == 'I':
		order = binary.LittleEndian
	case data[0] == 'M' && data[1] == 'M':
		order = binary.BigEndian
	default:
		return nil
	}
	if order.Uint16(data[2:4]) != 42 { // TIFF magic
		return nil
	}
	start := int(order.Uint32(data[4:8]))
	if start < 8 || start >= len(data) {
		return nil
	}

	var best []byte
	var walk func(off, depth int)
	walk = func(off, depth int) {
		if off < 8 || off+2 > len(data) || depth > 8 {
			return
		}
		count := int(order.Uint16(data[off : off+2]))
		if count > 512 {
			return
		}
		dir := off + 2
		for i := 0; i < count; i++ {
			e := dir + i*12
			if e+12 > len(data) {
				return
			}
			tag := order.Uint16(data[e : e+2])
			typ := order.Uint16(data[e+2 : e+4])
			cnt := order.Uint32(data[e+4 : e+8])
			valOff := e + 8
			switch tag {
			case 0x0201: // JPEGInterchangeFormat: absolute file offset (LONG)
				if typ == 4 && cnt == 1 && valOff+4 <= len(data) {
					o := int(order.Uint32(data[valOff : valOff+4]))
					if o > 0 && o < len(data) {
						if seg := extractJPEG(data[o:]); len(seg) > len(best) {
							best = seg
						}
					}
				}
			case 0x8769, 0x8825: // ExifIFD / GPS IFD pointer
				if typ == 4 && cnt == 1 && valOff+4 <= len(data) {
					walk(int(order.Uint32(data[valOff:valOff+4])), depth+1)
				}
			case 0x014A: // SubIFDs: array of IFD pointers
				if typ == 4 && cnt >= 1 && cnt <= 64 {
					var ptrs []uint32
					if int(cnt)*4 <= 4 {
						for k := uint32(0); k < cnt; k++ {
							ptrs = append(ptrs, order.Uint32(data[valOff+4*int(k):valOff+4*int(k)+4]))
						}
					} else {
						addr := int(order.Uint32(data[valOff : valOff+4]))
						for k := uint32(0); k < cnt; k++ {
							o := addr + 4*int(k)
							if o+4 > len(data) {
								break
							}
							ptrs = append(ptrs, order.Uint32(data[o:o+4]))
						}
					}
					for _, p := range ptrs {
						walk(int(p), depth+1)
					}
				}
			}
		}
		next := dir + count*12
		if next+4 <= len(data) {
			if n := int(order.Uint32(data[next : next+4])); n > 0 {
				walk(n, depth+1)
			}
		}
	}
	walk(start, 0)
	return best
}

// extractJPEG finds the largest valid JPEG segment in binary data.
func extractJPEG(data []byte) []byte {
	var best []byte
	pos := 0

	for pos < len(data)-1 {
		if data[pos] == 0xFF && data[pos+1] == 0xD8 {
			// Found SOI — look for EOI
			end := pos + 2
			for end < len(data)-1 {
				if data[end] == 0xFF && data[end+1] == 0xD9 {
					candidate := data[pos : end+2]
					if isValidJPEG(candidate) && len(candidate) > len(best) {
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

// isValidJPEG checks that data starts with SOI and contains essential
// markers (SOF + SOS) for browser rendering. Rejects false positives
// like giant APP5-only segments found in some Sony ARW files.
func isValidJPEG(data []byte) bool {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return false
	}

	var hasSOF, hasSOS bool
	i := 2
	for i < len(data)-1 {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		switch {
		case marker == 0x00: // stuffed byte
			i += 2
		case marker == 0xFF: // padding
			i++
		case marker == 0xD9: // EOI — stop scanning
			i += 2
		case marker >= 0xC0 && marker <= 0xCF && marker != 0xC4:
			hasSOF = true
			i += 2 + int(data[i+2])<<8 + int(data[i+3])
		case marker == 0xDA: // SOS
			hasSOS = true
			i += 2 + int(data[i+2])<<8 + int(data[i+3])
		case marker >= 0xD0 && marker <= 0xD7: // RST (no length)
			i += 2
		default:
			// Valid marker with length field
			if !isJPEGMarker(marker) {
				return false
			}
			if i+4 > len(data) {
				return false
			}
			i += 2 + int(data[i+2])<<8 + int(data[i+3])
		}
	}
	return hasSOF && hasSOS
}

// isJPEGMarker reports whether b is a valid JPEG marker byte.
func isJPEGMarker(b byte) bool {
	if b == 0xFE {
		return true
	}
	if b < 0xC0 {
		return false
	}
	if b == 0xD8 || b == 0xD9 {
		return false
	}
	if b >= 0xF0 {
		return false
	}
	return true
}
