package service

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// thumbTarget 是缩略图的长边目标：库内既有 6.8 万张 webp 抽样后长边集中在
// 1078/1080（少量 1280），v1 上传却按短边 1080 产出 1620×1080，比库内大 2.7 倍。
// v2 统一按长边 1080。
const thumbTarget = 1080

const (
	minPreviewEdge = 320  // 客户端预览最小边
	maxPreviewEdge = 1600 // 客户端预览最大边（长边应 ≈1080，留出余量）
)

// ThumbSource 标明缩略图实际来源，用于日志与统计。
type ThumbSource string

const (
	ThumbFromClientPreview ThumbSource = "client_preview"
	ThumbFromOriginal      ThumbSource = "original"
	ThumbFromEmbedded      ThumbSource = "embedded"
)

// rawExts 是需要走内嵌预览的 RAW 扩展名（cwebp 读不了这些容器）。
var rawExts = map[string]bool{
	".nef": true, ".cr2": true, ".cr3": true, ".arw": true, ".rw2": true,
	".dng": true, ".orf": true, ".raf": true, ".pef": true, ".sr2": true, ".srf": true,
}

func isRawExt(ext string) bool {
	return rawExts[strings.ToLower(ext)]
}

// thumbResizeArgs 生成 cwebp 的 -resize 参数。
// 长边已经 ≤ target 时返回 nil（绝不放大：小图保持原尺寸）。
// 横图缩宽 `-resize 1080 0`，竖/方图缩高 `-resize 0 1080`（0 表示按比例推算）。
func thumbResizeArgs(w, h, target int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	if w > h {
		if w <= target {
			return nil
		}
		return []string{"-resize", strconv.Itoa(target), "0"}
	}
	if h <= target {
		return nil
	}
	return []string{"-resize", "0", strconv.Itoa(target)}
}

// validateClientPreview 校验客户端送来的缩略图源（必须是能解出合理尺寸的 JPEG）。
// RAW 的内嵌预览常与传感器纵横比不同，因此 RAW 跳过比例校验。
func validateClientPreview(b []byte, ext string, exifW, exifH int) bool {
	if len(b) == 0 {
		return false
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return false
	}
	w, h := cfg.Width, cfg.Height
	if w < minPreviewEdge || h < minPreviewEdge || w > maxPreviewEdge || h > maxPreviewEdge {
		return false
	}
	if isRawExt(ext) || exifW <= 0 || exifH <= 0 {
		return true
	}
	ar := float64(w) / float64(h)
	ear := float64(exifW) / float64(exifH)
	if ear <= 0 {
		return true
	}
	return math.Abs(ar-ear)/ear <= 0.05
}

// renderThumbnail 三级降级生成 {id}.webp：
//
//  1. 客户端预览（长边 1080 的 JPEG）→ cwebp，0.14s/张（实测）
//  2. 原图直接 cwebp -resize，0.85s/张（cwebp 原生读 JPEG/PNG/TIFF/WebP）
//  3. exiftool 取 RAW/HEIC 内嵌预览 → cwebp -resize
//
// 三级都失败才返回错误；调用方据此只置 thumbnail=false，不阻断上传。
func (s *UploadService) renderThumbnail(ctx context.Context, srcAbs string, preview []byte, ex *PhotoEXIF, outPath string) (ThumbSource, error) {
	if err := os.MkdirAll(s.ThumbnailsDir, 0755); err != nil {
		return "", err
	}
	ext := filepath.Ext(srcAbs)

	if len(preview) > 0 && validateClientPreview(preview, ext, ex.Width, ex.Height) {
		if err := s.cwebpBytes(ctx, preview, thumbResizeArgs(ex.Width, ex.Height, thumbTarget), outPath); err == nil {
			return ThumbFromClientPreview, nil
		}
	}

	// dims 未知时不走原图路径：无法判断是否需要缩放，可能把 100MP 原图整张编码
	if !isRawExt(ext) && ex.Width > 0 && ex.Height > 0 {
		args := thumbResizeArgs(ex.Width, ex.Height, thumbTarget)
		if err := s.cwebpFile(ctx, srcAbs, args, outPath); err == nil {
			return ThumbFromOriginal, nil
		}
	}

	data, err := extractWithExiftool(srcAbs)
	if err != nil {
		return "", fmt.Errorf("thumbnail: %w", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("thumbnail: embedded preview: %w", err)
	}
	if err := s.cwebpBytes(ctx, data, thumbResizeArgs(cfg.Width, cfg.Height, thumbTarget), outPath); err != nil {
		return "", err
	}
	return ThumbFromEmbedded, nil
}

// cwebpBytes 把内存中的图片字节交给 cwebp 编码。
func (s *UploadService) cwebpBytes(ctx context.Context, data []byte, resizeArgs []string, outPath string) error {
	tmp, err := os.CreateTemp("", "ss_thumb_*.jpg")
	if err != nil {
		return err
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)
	if err := os.WriteFile(name, data, 0600); err != nil {
		return err
	}
	return s.cwebpFile(ctx, name, resizeArgs, outPath)
}

// cwebpFile 调用 cwebp 编码：先写 {out}.tmp 再 rename，保证对外可见的缩略图
// 永远是完整文件（cwebp 失败时不会留下半个 webp）。
func (s *UploadService) cwebpFile(ctx context.Context, src string, resizeArgs []string, outPath string) error {
	args := []string{"-quiet", "-q", "80", "-mt"}
	args = append(args, resizeArgs...)
	args = append(args, src, "-o", outPath+".tmp")
	cmd := exec.CommandContext(ctx, "cwebp", args...)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cwebp: %v: %s", err, data)
	}
	if err := os.Rename(outPath+".tmp", outPath); err != nil {
		os.Remove(outPath + ".tmp")
		return err
	}
	return nil
}
