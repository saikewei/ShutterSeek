package importer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shutterseek/tools/importer/internal/extractor"
	"shutterseek/tools/importer/internal/models"
)

// Stats tracks import statistics.
type Stats struct {
	Total   int64
	New     int64
	Updated int64
	Skipped int64
	Errors  int64
}

func (s *Stats) String() string {
	return fmt.Sprintf("total=%d new=%d updated=%d skipped=%d errors=%d",
		s.Total, s.New, s.Updated, s.Skipped, s.Errors)
}

var insertColumns = []string{
	"file_path", "file_hash", "file_size", "width", "height",
	"taken_at", "camera_make", "camera_model", "lens_model",
	"focal_length", "aperture", "iso", "status",
}

// Importer manages concurrent photo metadata extraction and database import.
type Importer struct {
	pool      *pgxpool.Pool
	extractor *extractor.Extractor
	workers   int
	batchSize int
	photosDir string
	stats     Stats
}

// Stats returns a copy of current statistics.
func (imp *Importer) Stats() Stats {
	return Stats{
		Total:   atomic.LoadInt64(&imp.stats.Total),
		New:     atomic.LoadInt64(&imp.stats.New),
		Updated: atomic.LoadInt64(&imp.stats.Updated),
		Skipped: atomic.LoadInt64(&imp.stats.Skipped),
		Errors:  atomic.LoadInt64(&imp.stats.Errors),
	}
}

func New(pool *pgxpool.Pool, workers, batchSize int, photosDir string) *Importer {
	return &Importer{
		pool:      pool,
		extractor: extractor.New(),
		workers:   workers,
		batchSize: batchSize,
		photosDir: photosDir,
	}
}

// Run processes file paths from the channel using a worker pool.
func (imp *Importer) Run(ctx context.Context, paths <-chan string) *Stats {
	var wg sync.WaitGroup

	for i := 0; i < imp.workers; i++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			imp.worker(ctx, wid, paths)
		}(i)
	}

	wg.Wait()
	return &imp.stats
}

func (imp *Importer) worker(ctx context.Context, wid int, paths <-chan string) {
	batch := make([]string, 0, imp.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		imp.processBatch(ctx, batch)
		batch = batch[:0]
	}

	for p := range paths {
		batch = append(batch, p)
		if len(batch) >= imp.batchSize {
			flush()
		}
	}
	flush()
}

func (imp *Importer) processBatch(ctx context.Context, paths []string) {
	// 1. Extract metadata
	metas, err := imp.extractor.ExtractBatch(paths)
	if err != nil {
		log.Printf("exiftool error: %v", err)
		atomic.AddInt64(&imp.stats.Errors, int64(len(paths)))
		return
	}

	// 2. Convert to DBPhotos
	extracted := make(map[string]*models.DBPhoto, len(metas))
	for _, m := range metas {
		p := m.ToDBPhoto(imp.photosDir)
		extracted[p.FilePath] = p
	}

	errors := int64(len(paths) - len(extracted))
	atomic.AddInt64(&imp.stats.Errors, errors)

	if len(extracted) == 0 {
		return
	}

	// 3. Collect unique paths
	relPaths := make([]string, 0, len(extracted))
	for fp := range extracted {
		relPaths = append(relPaths, fp)
	}

	// 4. Check existing records
	existing := imp.checkExisting(ctx, relPaths)

	// 5. Split into insert/update/skip
	var toInsert, toUpdate []*models.DBPhoto
	for fp, p := range extracted {
		if oldHash, ok := existing[fp]; ok {
			if oldHash != p.FileHash {
				toUpdate = append(toUpdate, p)
			}
			// else: hash same → skip
		} else {
			toInsert = append(toInsert, p)
		}
	}
	atomic.AddInt64(&imp.stats.Skipped, int64(len(extracted)-len(toInsert)-len(toUpdate)))

	// 6. Execute
	if len(toInsert) > 0 {
		n := imp.insertBatch(ctx, toInsert)
		atomic.AddInt64(&imp.stats.New, n)
	}
	if len(toUpdate) > 0 {
		n := imp.updateBatch(ctx, toUpdate)
		atomic.AddInt64(&imp.stats.Updated, n)
	}
	atomic.AddInt64(&imp.stats.Total, int64(len(extracted)))
}

func (imp *Importer) checkExisting(ctx context.Context, paths []string) map[string]string {
	rows, err := imp.pool.Query(ctx,
		`SELECT file_path, file_hash FROM photos WHERE file_path = ANY($1)`,
		paths,
	)
	if err != nil {
		log.Printf("checkExisting: %v", err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]string, len(paths))
	for rows.Next() {
		var fp, hash string
		if err := rows.Scan(&fp, &hash); err == nil {
			result[fp] = hash
		}
	}
	return result
}

func (imp *Importer) insertBatch(ctx context.Context, photos []*models.DBPhoto) int64 {
	tx, err := imp.pool.Begin(ctx)
	if err != nil {
		log.Printf("insert begin: %v", err)
		return 0
	}
	defer tx.Rollback(ctx)

	// Use pgx.CopyFromSlice for efficient bulk insert
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"photos"},
		insertColumns,
		pgx.CopyFromSlice(len(photos), func(i int) ([]any, error) {
			p := photos[i]
			return []any{
				p.FilePath, p.FileHash, p.FileSize,
				p.Width, p.Height, p.TakenAt,
				p.CameraMake, p.CameraModel, p.LensModel,
				p.FocalLength, p.Aperture, p.ISO, p.Status,
			}, nil
		}),
	)
	if err != nil {
		// If COPY fails (e.g. unique constraint), fall back to INSERT
		log.Printf("copy insert: %v — falling back to INSERT", err)
		return imp.insertBatchFallback(ctx, photos)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("insert commit: %v", err)
		return 0
	}
	return int64(len(photos))
}

func (imp *Importer) insertBatchFallback(ctx context.Context, photos []*models.DBPhoto) int64 {
	var inserted int64
	for _, p := range photos {
		_, err := imp.pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO photos (%s) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (file_path) DO NOTHING`,
			strings.Join(insertColumns, ",")),
			p.FilePath, p.FileHash, p.FileSize,
			p.Width, p.Height, p.TakenAt,
			p.CameraMake, p.CameraModel, p.LensModel,
			p.FocalLength, p.Aperture, p.ISO, p.Status,
		)
		if err == nil {
			inserted++
		}
	}
	return inserted
}

func (imp *Importer) updateBatch(ctx context.Context, photos []*models.DBPhoto) int64 {
	var updated int64
	for _, p := range photos {
		tag, err := imp.pool.Exec(ctx, `
			UPDATE photos SET
				file_hash=$2, file_size=$3, width=$4, height=$5,
				taken_at=$6, camera_make=$7, camera_model=$8, lens_model=$9,
				focal_length=$10, aperture=$11, iso=$12, status=$13
			WHERE file_path=$1`,
			p.FilePath, p.FileHash, p.FileSize,
			p.Width, p.Height, p.TakenAt,
			p.CameraMake, p.CameraModel, p.LensModel,
			p.FocalLength, p.Aperture, p.ISO, p.Status,
		)
		if err != nil {
			log.Printf("update %s: %v", p.FilePath, err)
			continue
		}
		updated += tag.RowsAffected()
	}
	return updated
}
