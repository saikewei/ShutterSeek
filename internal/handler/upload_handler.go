package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/model"
	"shutterseek/internal/service"
)

const maxUploadBytes = 200 << 20 // 200MB（单文件）

// 非文件 part 的上限：vector 是 1024 个 float 的 JSON（≈12KB），preview 是
// 长边 1080 的 JPEG（实测 170KB），这里各留足余量但绝不无界读内存。
const (
	maxVectorPartBytes  = 256 << 10
	maxPreviewPartBytes = 4 << 20
)

// Upload 处理 multipart 上传：file + vector。
// POST /api/v1/photos/upload（admin only）
func (h *Handler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
		return
	}
	vecStr := c.PostForm("vector")
	if vecStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing vector"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "open file"})
		return
	}
	defer f.Close()

	p, err := h.UploadSvc.Upload(c.Request.Context(), f, fileHeader.Filename, vecStr)
	switch {
	case errors.Is(err, service.ErrInvalidVector):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid vector"})
		return
	case err != nil:
		var dup service.ErrDuplicate
		if errors.As(err, &dup) {
			c.JSON(http.StatusConflict, gin.H{
				"duplicate":   true,
				"existing_id": dup.ExistingID,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":            p.ID,
		"file_path":     p.FilePath,
		"taken_at":      p.TakenAt.Format("2006-01-02T15:04:05Z07:00"),
		"width":         p.Width,
		"height":        p.Height,
		"thumbnail_url": fmt.Sprintf("/api/thumbnails/%d.webp", p.ID),
		"duplicate":     false,
	})
}

// ── 批量上传 ────────────────────────────────────────────

// batchPart 是某个下标累积到的分片内容。multipart 的各 part 顺序不定，
// 所以先按下标归位，再一次性处理。
type batchPart struct {
	hasFile bool
	name    string
	tmpPath string
	hash    string
	size    int64
	seen    bool // 收到过任意 part（用于识别「只有 vector、没有 file」的残缺项）
	vecRaw  string
	preview []byte
}

// UploadBatch 处理批量上传：file_<i> + vector_<i> + 可选 preview_<i>。
// POST /api/v1/photos/upload/batch（admin only）
//
// 顺序流式解析（MultipartReader，不用 ParseMultipartForm）：文件边读边算
// SHA-256 直接落 UploadDir 临时文件，不经过 /tmp、不把整批读进内存。
// 只要 multipart 可解析就返回 200，逐项状态在结果里（部分成功）。
func (h *Handler) UploadBatch(c *gin.Context) {
	opts := h.UploadSvc.Options()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, opts.MaxBatchBytes)

	mr, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart body"})
		return
	}

	parts := map[int]*batchPart{}
	defer func() {
		for _, p := range parts {
			if p.tmpPath != "" {
				os.Remove(p.tmpPath)
			}
		}
	}()

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "batch too large"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "read multipart: " + err.Error()})
			return
		}

		name := p.FormName()
		if name == "" {
			p.Close()
			continue
		}
		idx, kind, ok := parseBatchPartName(name)
		if !ok {
			p.Close()
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown part: " + name})
			return
		}
		if idx >= opts.BatchMaxFiles {
			p.Close()
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("part index %d exceeds batch limit %d", idx, opts.BatchMaxFiles)})
			return
		}

		bp := parts[idx]
		if bp == nil {
			bp = &batchPart{}
			parts[idx] = bp
		}
		if bp.seen && kind == "file" {
			p.Close()
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("duplicate part file_%d", idx)})
			return
		}
		bp.seen = true

		switch kind {
		case "file":
			if err := h.readFilePart(p, bp); err != nil {
				p.Close()
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "batch too large"})
					return
				}
				if errors.Is(err, errFileTooLarge) {
					c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file_" + strconv.Itoa(idx) + " exceeds 200MB"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "store file: " + err.Error()})
				return
			}
		case "vector":
			b, err := io.ReadAll(io.LimitReader(p, maxVectorPartBytes+1))
			if err != nil || len(b) > maxVectorPartBytes {
				bp.vecRaw = ""
			} else {
				bp.vecRaw = string(b)
			}
		case "preview":
			b, err := io.ReadAll(io.LimitReader(p, maxPreviewPartBytes+1))
			// 超限/读失败都当作「没有预览」→ 服务端自己兜底，不让整批失败
			if err == nil && len(b) <= maxPreviewPartBytes {
				bp.preview = b
			}
		}
		p.Close()
	}

	// 归位：有 file 的进批处理，只有 vector/preview 的算残缺项
	items := make([]service.UploadItem, 0, len(parts))
	orphans := make([]service.UploadResult, 0)
	for idx, bp := range parts {
		if !bp.hasFile {
			orphans = append(orphans, service.UploadResult{
				Index: idx, Filename: bp.name, Status: service.StatusError, Err: "missing file",
			})
			continue
		}
		vec, verr := service.ParseVector(bp.vecRaw)
		if verr != nil {
			vec = nil // 交给 service 判成 invalid vector，保持单一判定入口
		}
		items = append(items, service.UploadItem{
			Index: idx, Filename: bp.name, TmpPath: bp.tmpPath,
			Hash: bp.hash, Size: bp.size, Vector: vec, Preview: bp.preview,
		})
	}
	if len(items) == 0 && len(orphans) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file parts"})
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Index < items[j].Index })

	start := time.Now()
	results, sum := h.UploadSvc.UploadBatch(c.Request.Context(), items)
	elapsed := time.Since(start)
	results = append(results, orphans...)
	sort.Slice(results, func(i, j int) bool { return results[i].Index < results[j].Index })

	log.Printf("upload batch files=%d created=%d dup=%d failed=%d thumb_ms=%d elapsed_ms=%d",
		len(items)+len(orphans), sum.Created, sum.Duplicate, sum.Failed, sum.SumThumbMs, elapsed.Milliseconds())

	if sum.Created > 0 && h.AuthSvc != nil {
		if uid, ok := c.Get("user_id"); ok {
			if id, ok := uid.(int64); ok {
				username := ""
				if u, err := h.AuthSvc.FindUserByID(id); err == nil && u != nil {
					username = u.Username
				}
				h.AuthSvc.LogEvent(id, username, model.LogEventUpload, c.ClientIP())
			}
		}
	}

	out := make([]gin.H, 0, len(results))
	for _, r := range results {
		item := gin.H{
			"index":     r.Index,
			"filename":  r.Filename,
			"status":    r.Status,
			"duplicate": r.Status == service.StatusDuplicate,
		}
		switch r.Status {
		case service.StatusCreated:
			p := r.Photo
			item["id"] = p.ID
			item["file_path"] = p.FilePath
			item["taken_at"] = formatTime(p.TakenAt)
			item["width"] = p.Width
			item["height"] = p.Height
			item["thumbnail_url"] = fmt.Sprintf("/api/thumbnails/%d.webp", p.ID)
			item["thumbnail"] = r.ThumbnailOK
		case service.StatusDuplicate:
			item["existing_id"] = r.ExistingID
		default:
			item["error"] = r.Err
		}
		out = append(out, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"results": out,
		"summary": gin.H{
			"created":    sum.Created,
			"duplicate":  sum.Duplicate,
			"failed":     sum.Failed,
			"elapsed_ms": elapsed.Milliseconds(),
		},
	})
}

var errFileTooLarge = errors.New("file too large")

// readFilePart 把 file part 流式写入 UploadDir 的临时文件，同时算 SHA-256。
func (h *Handler) readFilePart(p *multipart.Part, bp *batchPart) error {
	if err := os.MkdirAll(h.UploadSvc.UploadDir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(h.UploadSvc.UploadDir, ".upload-*")
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(p, service.DefaultMaxSingleBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if n > service.DefaultMaxSingleBytes {
		os.Remove(tmp.Name())
		return errFileTooLarge
	}
	bp.hasFile = true
	bp.name = p.FileName()
	bp.tmpPath = tmp.Name()
	bp.size = n
	bp.hash = hex.EncodeToString(hash.Sum(nil))
	return nil
}

// parseBatchPartName 解析 `file_3` / `vector_3` / `preview_3`。
func parseBatchPartName(name string) (int, string, bool) {
	for _, kind := range []string{"file", "vector", "preview"} {
		rest, ok := strings.CutPrefix(name, kind+"_")
		if !ok {
			continue
		}
		idx, err := strconv.Atoi(rest)
		if err != nil || idx < 0 {
			return 0, "", false
		}
		return idx, kind, true
	}
	return 0, "", false
}
