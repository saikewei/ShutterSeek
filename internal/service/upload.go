package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/image/draw"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

type ErrDuplicate struct {
	ExistingID int64
}

func (e ErrDuplicate) Error() string {
	return fmt.Sprintf("duplicate photo id=%d", e.ExistingID)
}

type UploadService struct {
	DB            *gorm.DB
	Redis         *goredis.Client
	UploadDir     string
	ThumbnailsDir string
}

func NewUploadService(db *gorm.DB, rdb *goredis.Client, uploadDir, thumbnailsDir string) *UploadService {
	return &UploadService{DB: db, Redis: rdb, UploadDir: uploadDir, ThumbnailsDir: thumbnailsDir}
}

// Upload 接收原始文件流与客户端向量，完成落盘、查重、事务落库、缩略图与缓存失效。
func (s *UploadService) Upload(ctx context.Context, src io.Reader, origName, vecStr string) (*model.Photo, error) {
	vec, err := ParseVector(vecStr)
	if err != nil {
		return nil, ErrInvalidVector
	}

	// 1. 流式写临时文件并同时算 SHA-256
	if err := os.MkdirAll(s.UploadDir, 0755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(s.UploadDir, ".upload-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), src); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	hashHex := hex.EncodeToString(h.Sum(nil))

	// 2. SHA-256 查重
	var existing model.Photo
	err = s.DB.WithContext(ctx).Where("file_hash = ?", hashHex).First(&existing).Error
	if err == nil {
		return nil, ErrDuplicate{ExistingID: existing.ID}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 3. EXIF → 拍摄时间与元数据
	ex, err := extractEXIF(tmpName)
	if err != nil {
		return nil, err
	}
	takenAt := time.Now()
	if ex.TakenAt != nil {
		takenAt = *ex.TakenAt
	}

	// 4. 目标路径（临时文件先 rename 再入库，失败即删）。
	// DB 里 uploads/ 只是命名空间前缀，落盘时剥掉，UploadDir 即年/月的根。
	rel := buildUploadRelPath(takenAt, origName, hashHex[:8])
	abs := uploadAbsPath(s.UploadDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(abs)
		}
	}()

	fi, _ := os.Stat(abs)
	var size int64
	if fi != nil {
		size = fi.Size()
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

	// 5. 事务：photos + photo_embeddings
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO photo_embeddings (photo_id, embedding) VALUES (?, ?::vector)`,
			p.ID, formatVector(vec),
		).Error
	})
	if err != nil {
		return nil, err
	}
	committed = true

	// 6. 缩略图（失败不阻断上传）
	if err := s.generateThumbnail(ctx, p.ID, abs); err != nil {
		log.Printf("thumbnail failed id=%d: %v", p.ID, err)
	}

	// 7. 失效缓存
	s.invalidateCaches(ctx)

	return p, nil
}

// uploadAbsPath 把 DB 相对路径（uploads/...）映射到上传根目录的绝对路径。
func uploadAbsPath(uploadDir, rel string) string {
	return filepath.Join(uploadDir, strings.TrimPrefix(rel, "uploads/"))
}

// resizeShort 按短边缩放到 size（保持宽高比，ApproxBiLinear）。
func resizeShort(img image.Image, size int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	short := w
	if h < w {
		short = h
	}
	scale := float64(size) / float64(short)
	nw := int(math.Round(float64(w) * scale))
	nh := int(math.Round(float64(h) * scale))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// generateThumbnail 生成 {id}.webp：Go 可解码 → 解码缩放为临时 JPEG；
// 否则 exiftool 提取内嵌预览；最后 cwebp 编码。
func (s *UploadService) generateThumbnail(ctx context.Context, id int64, absPath string) error {
	tmpJPG, err := s.preparePreviewJPG(absPath)
	if err != nil {
		return err
	}
	defer os.Remove(tmpJPG)
	out := filepath.Join(s.ThumbnailsDir, fmt.Sprintf("%d.webp", id))
	if err := os.MkdirAll(s.ThumbnailsDir, 0755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "cwebp", "-quiet", "-q", "80", "-mt", tmpJPG, "-o", out)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cwebp: %v: %s", err, data)
	}
	return nil
}

// preparePreviewJPG 把原图变成短边 ≤1080 的临时 JPEG（供 cwebp 编码）。
func (s *UploadService) preparePreviewJPG(absPath string) (string, error) {
	tmp, err := os.CreateTemp("", "ss_thumb_*.jpg")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer tmp.Close()

	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		// RAW/HEIC：exiftool 提取内嵌 JPEG 预览
		data, eerr := exec.Command("exiftool", "-b", "-PreviewImage", absPath).Output()
		if eerr != nil || len(data) < minPreviewSize {
			return "", fmt.Errorf("decode and exiftool preview failed: %v", eerr)
		}
		return name, os.WriteFile(name, data, 0644)
	}
	// 短边超过 1080 才缩放，小图保持原尺寸（避免放大损失画质）
	b := img.Bounds()
	short := b.Dx()
	if b.Dy() < short {
		short = b.Dy()
	}
	if short > 1080 {
		img = resizeShort(img, 1080)
	}
	return name, jpeg.Encode(tmp, img, &jpeg.Options{Quality: 92})
}

// invalidateCaches 清除上传后受影响的 Redis 缓存。
func (s *UploadService) invalidateCaches(ctx context.Context) {
	if s.Redis == nil {
		return
	}
	s.Redis.Del(ctx, "cache:total_photos")
	for _, pat := range []string{"cache:first_page:*", "cache:photo_dates*"} {
		var cursor uint64
		for {
			keys, next, err := s.Redis.Scan(ctx, cursor, pat, 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				s.Redis.Del(ctx, keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
}
