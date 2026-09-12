package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// UploadResult.Status 取值。
const (
	StatusCreated   = "created"
	StatusDuplicate = "duplicate"
	StatusError     = "error"
)

// 默认限额。batch 上限同时受 handler 的 MaxBytesReader 与这里的下标上限约束。
const (
	DefaultUploadWorkers  = 2
	DefaultBatchMaxFiles  = 8
	DefaultMaxBatchBytes  = int64(512) << 20
	DefaultMaxSingleBytes = int64(200) << 20
)

type ErrDuplicate struct {
	ExistingID int64
}

func (e ErrDuplicate) Error() string {
	return fmt.Sprintf("duplicate photo id=%d", e.ExistingID)
}

// UploadOptions 控制重活并发与批次限额。
// Workers 是**进程级**闸门：N 个并发请求共享它。否则每个请求各开一组
// 解码/缩略图任务，NAS 的 4 核 + 8GB 会被打爆。
type UploadOptions struct {
	Workers       int
	BatchMaxFiles int
	MaxBatchBytes int64
}

func DefaultUploadOptions() UploadOptions {
	return UploadOptions{
		Workers:       DefaultUploadWorkers,
		BatchMaxFiles: DefaultBatchMaxFiles,
		MaxBatchBytes: DefaultMaxBatchBytes,
	}
}

func (o UploadOptions) normalized() UploadOptions {
	if o.Workers < 1 {
		o.Workers = DefaultUploadWorkers
	}
	if o.BatchMaxFiles < 1 {
		o.BatchMaxFiles = DefaultBatchMaxFiles
	}
	if o.MaxBatchBytes <= 0 {
		o.MaxBatchBytes = DefaultMaxBatchBytes
	}
	return o
}

type UploadService struct {
	DB            *gorm.DB
	Redis         *goredis.Client
	UploadDir     string
	ThumbnailsDir string

	opts        UploadOptions
	heavy       chan struct{}
	invalidator *CacheInvalidator
}

// NewUploadService 保持 v1 的四参签名（默认并发/限额）。
func NewUploadService(db *gorm.DB, rdb *goredis.Client, uploadDir, thumbnailsDir string) *UploadService {
	return NewUploadServiceWithOptions(db, rdb, uploadDir, thumbnailsDir, DefaultUploadOptions())
}

func NewUploadServiceWithOptions(db *gorm.DB, rdb *goredis.Client, uploadDir, thumbnailsDir string, opts UploadOptions) *UploadService {
	opts = opts.normalized()
	return &UploadService{
		DB:            db,
		Redis:         rdb,
		UploadDir:     uploadDir,
		ThumbnailsDir: thumbnailsDir,
		opts:          opts,
		heavy:         make(chan struct{}, opts.Workers),
		invalidator:   NewCacheInvalidator(&Cache{Redis: rdb}, DefaultInvalidateInterval),
	}
}

// Options 暴露生效后的限额（handler 据此限制批次）。
func (s *UploadService) Options() UploadOptions { return s.opts }

// Invalidate 让上传服务持有的缓存失效器立即可用（进程启动时调用）。
func (s *UploadService) Start() {
	if s.invalidator != nil {
		s.invalidator.Start()
	}
}

// Stop 停掉后台失效 ticker 并清掉未落地的失效请求。
func (s *UploadService) Stop() {
	if s.invalidator != nil {
		s.invalidator.FlushNow()
		s.invalidator.Stop()
	}
}

func (s *UploadService) acquire() {
	if s.heavy == nil {
		return
	}
	s.heavy <- struct{}{}
}

func (s *UploadService) release() {
	if s.heavy == nil {
		return
	}
	<-s.heavy
}

// UploadItem 是一张待入库的照片。TmpPath 由 handler 流式写入（顺带算好 Hash）。
type UploadItem struct {
	Index    int
	Filename string
	TmpPath  string
	Hash     string // 可空：空则本服务计算
	Size     int64
	Vector   []float32
	Preview  []byte // 可空：客户端生成的缩略图源
}

// UploadResult 是单张照片的结局。Status 恒为 created/duplicate/error 之一。
type UploadResult struct {
	Index       int
	Filename    string
	Status      string
	Photo       *model.Photo // created 时非 nil
	ExistingID  int64        // duplicate 时
	ThumbnailOK bool
	ThumbSource string
	ThumbMs     int64
	Err         string
}

type UploadSummary struct {
	Created    int
	Duplicate  int
	Failed     int
	SumThumbMs int64
}

// UploadBatch 并发处理一批照片：重活（EXIF/缩略图）受进程级闸门限流，
// 单项失败不影响其它项（部分成功）。
func (s *UploadService) UploadBatch(ctx context.Context, items []UploadItem) ([]UploadResult, UploadSummary) {
	results := make([]UploadResult, len(items))
	if len(items) == 0 {
		return results, UploadSummary{}
	}

	// EXIF 一次调用批量化：逐张 spawn exiftool 是纯浪费（实测 176ms/张 vs 62ms/张）
	paths := make([]string, 0, len(items))
	for _, it := range items {
		if it.TmpPath != "" {
			paths = append(paths, it.TmpPath)
		}
	}
	s.acquire()
	exifs, exifErr := extractEXIFMany(paths)
	s.release()
	if exifErr != nil {
		log.Printf("upload: exiftool batch failed: %v", exifErr)
	}

	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = s.uploadItem(ctx, items[i], exifs[items[i].TmpPath], exifErr)
		}(i)
	}
	wg.Wait()

	sum := summarizeUpload(results)
	if sum.Created > 0 {
		s.invalidateCaches()
	}
	return results, sum
}

// summarizeUpload 纯函数：逐项结果 → 计数汇总。
func summarizeUpload(results []UploadResult) UploadSummary {
	var sum UploadSummary
	for _, r := range results {
		switch r.Status {
		case StatusCreated:
			sum.Created++
			sum.SumThumbMs += r.ThumbMs
		case StatusDuplicate:
			sum.Duplicate++
		default:
			sum.Failed++
		}
	}
	return sum
}

func (s *UploadService) uploadItem(ctx context.Context, it UploadItem, ex *PhotoEXIF, exifErr error) UploadResult {
	res := UploadResult{Index: it.Index, Filename: it.Filename}

	cleanupTmp := func() {
		if it.TmpPath != "" {
			os.Remove(it.TmpPath)
		}
	}
	fail := func(msg string) UploadResult {
		res.Status = StatusError
		res.Err = msg
		cleanupTmp()
		return res
	}

	if len(it.Vector) != 1024 {
		return fail("invalid vector")
	}
	if it.TmpPath == "" {
		return fail("missing file")
	}
	if ex == nil {
		if exifErr != nil {
			return fail("exif: " + exifErr.Error())
		}
		return fail("exif: no metadata")
	}

	hashHex := it.Hash
	if hashHex == "" {
		h, err := hashFile(it.TmpPath)
		if err != nil {
			return fail("hash: " + err.Error())
		}
		hashHex = h
	}

	// 快路径查重：绝大多数重复项在这里就结束，不必落盘
	var existing model.Photo
	switch err := s.DB.WithContext(ctx).Select("id").Where("file_hash = ?", hashHex).First(&existing).Error; {
	case err == nil:
		res.Status = StatusDuplicate
		res.ExistingID = existing.ID
		cleanupTmp()
		return res
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return fail("dedupe: " + err.Error())
	}

	takenAt := time.Now()
	if ex.TakenAt != nil {
		takenAt = *ex.TakenAt
	}
	rel := buildUploadRelPath(takenAt, it.Filename, hashHex[:8])
	abs := uploadAbsPath(s.UploadDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return fail("mkdir: " + err.Error())
	}
	if err := os.Rename(it.TmpPath, abs); err != nil {
		return fail("rename: " + err.Error())
	}

	committed := false
	keep := false
	defer func() {
		// duplicate 时目标路径可能是另一并发请求刚提交的同一份文件，绝不能删
		if !committed && !keep {
			os.Remove(abs)
		}
	}()

	size := it.Size
	if size == 0 {
		if fi, err := os.Stat(abs); err == nil {
			size = fi.Size()
		}
	}
	p := &model.Photo{
		FilePath:    rel,
		FileHash:    hashHex,
		FileSize:    size,
		Width:       int32(ex.Width),
		Height:      int32(ex.Height),
		TakenAt:     takenAt,
		CameraMake:  ex.CameraMake,
		CameraModel: ex.CameraModel,
		LensModel:   ex.LensModel,
		FocalLength: ex.FocalLength,
		Aperture:    ex.Aperture,
		Iso:         int32(ex.ISO),
		Status:      1,
	}

	if err := s.commitPhoto(ctx, p, it.Vector); err != nil {
		var dup ErrDuplicate
		if errors.As(err, &dup) {
			res.Status = StatusDuplicate
			res.ExistingID = dup.ExistingID
			// 只有当我们 rename 到的正是已入库那一行的路径时才能保留文件
			// （并发上传同一份内容、文件名也相同的情形：磁盘上是同一条路径，
			// 删掉会把赢家的文件删了）。文件名不同的重复项是孤儿文件，必须清掉。
			var winner model.Photo
			if e := s.DB.WithContext(ctx).Select("file_path").Where("id = ?", dup.ExistingID).First(&winner).Error; e == nil {
				keep = uploadAbsPath(s.UploadDir, winner.FilePath) == abs
			}
			return res
		}
		return fail("insert: " + err.Error())
	}
	committed = true

	// 缩略图在闸门内生成：失败不阻断上传，只标记 thumbnail=false
	t0 := time.Now()
	s.acquire()
	thumbSrc, terr := s.renderThumbnail(ctx, abs, it.Preview, ex, filepath.Join(s.ThumbnailsDir, fmt.Sprintf("%d.webp", p.ID)))
	s.release()
	res.ThumbMs = time.Since(t0).Milliseconds()
	if terr != nil {
		log.Printf("thumbnail failed id=%d: %v", p.ID, terr)
	} else {
		res.ThumbnailOK = true
		res.ThumbSource = string(thumbSrc)
	}

	res.Status = StatusCreated
	res.Photo = p
	return res
}

// commitPhoto 落库：photos + photo_embeddings 一个事务。
//
// 事务内先对 file_hash 取 advisory lock，再重查一次：批量/多客户端并发上传同一
// 文件时，后到者一定看到先到者提交的行，不会插出重复。错误码 23505（file_path
// 唯一约束 / 未来的 hash 唯一索引）也回查成 duplicate。
func (s *UploadService) commitPhoto(ctx context.Context, p *model.Photo, vec []float32) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", p.FileHash).Error; err != nil {
			return err
		}
		var existing model.Photo
		switch err := tx.Select("id").Where("file_hash = ?", p.FileHash).First(&existing).Error; {
		case err == nil:
			return ErrDuplicate{ExistingID: existing.ID}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO photo_embeddings (photo_id, embedding) VALUES (?, ?::vector)`,
			p.ID, formatVector(vec),
		).Error
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			var existing model.Photo
			if e := s.DB.WithContext(ctx).Select("id").Where("file_hash = ?", p.FileHash).First(&existing).Error; e == nil {
				return ErrDuplicate{ExistingID: existing.ID}
			}
		}
		return err
	}
	return nil
}

// Upload 是 v1 的单文件入口（保持签名与错误语义），内部走同一个批处理路径。
func (s *UploadService) Upload(ctx context.Context, src io.Reader, origName, vecStr string) (*model.Photo, error) {
	vec, err := ParseVector(vecStr)
	if err != nil {
		return nil, ErrInvalidVector
	}
	if err := os.MkdirAll(s.UploadDir, 0755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(s.UploadDir, ".upload-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return nil, err
	}
	var size int64
	if fi, err := os.Stat(tmpName); err == nil {
		size = fi.Size()
	}

	results, _ := s.UploadBatch(ctx, []UploadItem{{
		Index:    0,
		Filename: origName,
		TmpPath:  tmpName,
		Hash:     hex.EncodeToString(h.Sum(nil)),
		Size:     size,
		Vector:   vec,
	}})
	r := results[0]
	switch r.Status {
	case StatusCreated:
		return r.Photo, nil
	case StatusDuplicate:
		return nil, ErrDuplicate{ExistingID: r.ExistingID}
	default:
		if r.Err == "invalid vector" {
			return nil, ErrInvalidVector
		}
		return nil, errors.New(r.Err)
	}
}

// hashFile 计算文件的 SHA-256（十六进制小写）。
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// uploadAbsPath 把 DB 相对路径（uploads/...）映射到上传根目录的绝对路径。
func uploadAbsPath(uploadDir, rel string) string {
	return filepath.Join(uploadDir, strings.TrimPrefix(rel, "uploads/"))
}

// invalidateCaches 登记一次缓存失效（窗口内合并，避免每张都 SCAN 一遍）。
func (s *UploadService) invalidateCaches() {
	if s.invalidator == nil {
		return
	}
	s.invalidator.Request()
}
