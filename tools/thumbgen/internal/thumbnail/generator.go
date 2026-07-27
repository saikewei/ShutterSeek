package thumbnail

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "golang.org/x/image/tiff"
	"golang.org/x/image/draw"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Stats struct {
	Total        int64
	Generated    int64
	Skipped      int64
	SmallKept    int64
	StripSkipped int64 // full film strip (extreme aspect ratio)
	Errors       int64
}

func (s *Stats) String() string {
	return fmt.Sprintf("total=%d gen=%d skip=%d small=%d strip=%d err=%d",
		s.Total, s.Generated, s.Skipped, s.SmallKept, s.StripSkipped, s.Errors)
}

type job struct {
	id   int64
	path string
}

type Generator struct {
	pool      *pgxpool.Pool
	outputDir string
	size      int
	photosDir string
	workers   int
	stats     Stats
}

func New(pool *pgxpool.Pool, outputDir string, size int, photosDir string, workers int) *Generator {
	outputDir = filepath.Join(outputDir, fmt.Sprintf(".thumbnails_%d", size))
	return &Generator{
		pool:      pool,
		outputDir: outputDir,
		size:      size,
		photosDir: photosDir,
		workers:   workers,
	}
}

func (g *Generator) Stats() Stats {
	return Stats{
		Total:        atomic.LoadInt64(&g.stats.Total),
		Generated:    atomic.LoadInt64(&g.stats.Generated),
		Skipped:      atomic.LoadInt64(&g.stats.Skipped),
		SmallKept:    atomic.LoadInt64(&g.stats.SmallKept),
		StripSkipped: atomic.LoadInt64(&g.stats.StripSkipped),
		Errors:       atomic.LoadInt64(&g.stats.Errors),
	}
}

// Run processes all photos. limit=0 means all. startID=0 means from beginning.
func (g *Generator) Run(limit int, startID int64) (s *Stats) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC: %v\n%s", r, debug.Stack())
			s = &g.stats
		}
	}()
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		log.Fatalf("create dir: %v", err)
	}
	log.Printf("缩略图目录: %s", g.outputDir)
	log.Printf("并发数: %d", g.workers)

	query := `SELECT id, file_path FROM photos WHERE id > %d ORDER BY id`
	if limit > 0 {
		query = fmt.Sprintf(`SELECT id, file_path FROM photos WHERE id > %d ORDER BY id LIMIT %d`, startID, limit)
	} else {
		query = fmt.Sprintf(`SELECT id, file_path FROM photos WHERE id > %d ORDER BY id`, startID)
	}

	rows, err := g.pool.Query(context.Background(), query)
	if err != nil {
		log.Fatalf("query: %v", err)
	}

	var rawJobs, regularJobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.path); err != nil {
			continue
		}
		if isRaw(filepath.Ext(j.path)) {
			rawJobs = append(rawJobs, j)
		} else {
			regularJobs = append(regularJobs, j)
		}
	}
	rows.Close()

	log.Printf("RAW: %d, 普通: %d", len(rawJobs), len(regularJobs))
	totalJobs := len(rawJobs) + len(regularJobs)
	if totalJobs == 0 {
		return &g.stats
	}

	// Progress reporter goroutine
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		lastTotal := int64(0)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				s := g.Stats()
				delta := s.Total - lastTotal
				lastTotal = s.Total
				log.Printf("进度: %d/%d (%.1f%%) | +%d/30s | %s",
					s.Total, totalJobs,
					float64(s.Total)/float64(totalJobs)*100,
					delta, s.String())
			}
		}
	}()

	// Process regular images with worker pool
	jobs := make(chan job, 1000)
	var wg sync.WaitGroup
	for i := 0; i < g.workers; i++ {
		wg.Add(1)
		go func(wid int) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[reg-w%d] PANIC: %v\n%s", wid, r, debug.Stack())
				}
				wg.Done()
			}()
			g.regularWorker(jobs)
		}(i)
	}
	for _, j := range regularJobs {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	// Process RAW files — one at a time, each with its own exiftool call
	g.processRawSimple(rawJobs)

	return &g.stats
}

func (g *Generator) regularWorker(jobs <-chan job) {
	for j := range jobs {
		t0 := time.Now()
		g.processOne(j, g.fullPath(j.path))
		dt := time.Since(t0)
		if dt > 5*time.Second {
			log.Printf("[SLOW reg %ds] id=%d path=%s", int(dt.Seconds()), j.id, j.path)
		}
		atomic.AddInt64(&g.stats.Total, 1)
	}
}

// processRawSimple extracts previews from RAW files one at a time.
func (g *Generator) processRawSimple(rawJobs []job) {
	jobs := make(chan job, len(rawJobs))
	var wg sync.WaitGroup

	for i := 0; i < g.workers; i++ {
		wg.Add(1)
		go func(wid int) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[raw-w%d] PANIC: %v\n%s", wid, r, debug.Stack())
				}
				wg.Done()
			}()
			for j := range jobs {
				previewPath := g.extractOne(j)
				if previewPath == "" {
					atomic.AddInt64(&g.stats.Errors, 1)
					atomic.AddInt64(&g.stats.Total, 1)
					continue
				}
				g.processOne(j, previewPath)
				atomic.AddInt64(&g.stats.Total, 1)
			}
		}(i)
	}

	for _, j := range rawJobs {
		jobs <- j
	}
	close(jobs)
	wg.Wait()
}

// extractOne extracts the embedded JPEG preview from a single RAW file.
func (g *Generator) extractOne(j job) string {
	outPath := filepath.Join("/tmp", fmt.Sprintf("ss_preview_%d.jpg", j.id))
	src := g.fullPath(j.path)

	data, err := exec.Command("exiftool", "-b", "-PreviewImage", src).Output()
	if err == nil && len(data) > 0 {
		if err := os.WriteFile(outPath, data, 0644); err == nil {
			return outPath
		}
	}

	data, err = exec.Command("exiftool", "-b", "-JpgFromRaw", src).Output()
	if err == nil && len(data) > 0 {
		if err := os.WriteFile(outPath, data, 0644); err == nil {
			return outPath
		}
	}

	return ""
}

func (g *Generator) processOne(j job, imgPath string) {
	outPath := filepath.Join(g.outputDir, fmt.Sprintf("%d.jpg", j.id))

	// Skip if already exists
	if _, err := os.Stat(outPath); err == nil {
		atomic.AddInt64(&g.stats.Skipped, 1)
		if strings.HasPrefix(imgPath, os.TempDir()) {
			os.Remove(imgPath)
		}
		return
	}

	// Check file size
	fi, err := os.Stat(imgPath)
	if err != nil {
		atomic.AddInt64(&g.stats.Errors, 1)
		return
	}

	// Read image header (fast, no full decode)
	f, err := os.Open(imgPath)
	if err != nil {
		atomic.AddInt64(&g.stats.Errors, 1)
		return
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil {
		atomic.AddInt64(&g.stats.Errors, 1)
		return
	}
	w, h := int64(cfg.Width), int64(cfg.Height)
	tooLarge := fi.Size() > 200*1024*1024 || w*h > 50_000_000

	// Skip extreme aspect ratio film strips
	if w > 0 && h > 0 {
		ratio := float64(w) / float64(h)
		if ratio > 2.5 || ratio < 0.4 {
			atomic.AddInt64(&g.stats.StripSkipped, 1)
			if strings.HasPrefix(imgPath, os.TempDir()) {
				os.Remove(imgPath)
			}
			return
		}
	}

	// Skip huge files — no usable thumbnail possible
	if tooLarge {
		atomic.AddInt64(&g.stats.Errors, 1)
		if strings.HasPrefix(imgPath, os.TempDir()) {
			os.Remove(imgPath)
		}
		return
	}

	short := int(w)
	if int(h) < short {
		short = int(h)
	}

	// If image is already ≤ target size, copy directly
	if short <= g.size {
		if err := g.copyFile(imgPath, outPath); err != nil {
			atomic.AddInt64(&g.stats.Errors, 1)
			return
		}
		atomic.AddInt64(&g.stats.SmallKept, 1)
		if strings.HasPrefix(imgPath, os.TempDir()) {
			os.Remove(imgPath)
		}
		return
	}

	// Full decode for images needing resize
	f2, err := os.Open(imgPath)
	if err != nil {
		atomic.AddInt64(&g.stats.Errors, 1)
		return
	}
	img, _, err := image.Decode(f2)
	f2.Close()
	if err != nil {
		atomic.AddInt64(&g.stats.Errors, 1)
		return
	}

	resized := g.resize(img)
	if err := g.saveJPEG(resized, outPath); err != nil {
		atomic.AddInt64(&g.stats.Errors, 1)
		return
	}
	atomic.AddInt64(&g.stats.Generated, 1)

	if strings.HasPrefix(imgPath, os.TempDir()) {
		os.Remove(imgPath)
	}
}

func (g *Generator) copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func (g *Generator) resize(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	short := w
	if h < w {
		short = h
	}
	scale := float64(g.size) / float64(short)
	newW := int(math.Round(float64(w) * scale))
	newH := int(math.Round(float64(h) * scale))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}

func (g *Generator) saveJPEG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 85})
}

func (g *Generator) fullPath(relPath string) string {
	return filepath.Join(g.photosDir, relPath)
}

func isRaw(ext string) bool {
	switch strings.ToLower(ext) {
	case ".arw", ".nef", ".rw2", ".dng", ".cr2", ".cr3",
		".orf", ".raf", ".pef", ".sr2", ".srf":
		return true
	}
	return false
}
