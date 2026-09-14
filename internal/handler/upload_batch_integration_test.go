//go:build integration

package handler

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

// ── 素材与工具 ──────────────────────────────────────────

// uniqueJPEG 生成一张 seed 相关的 JPEG（不同 seed 内容/哈希必然不同，
// 避免测试之间因为同哈希被判为重复）。
func uniqueJPEG(t *testing.T, w, h, seed int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x + seed) % 256),
				G: uint8((y + seed*7) % 256),
				B: uint8((x*y + seed*13) % 256),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uniquePNG(t *testing.T, w, h, seed int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8((x + seed) % 256), G: uint8(y % 256), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// batchPartSpec 描述一个下标要发的 part。
type batchPartSpec struct {
	index   int
	name    string
	data    []byte
	vector  []float32 // nil 表示故意不发合法 vector
	preview []byte
}

// buildBatchRequest 组装批量上传请求（不依赖 *testing.T，便于并发测试复用）。
func buildBatchRequest(specs []batchPartSpec) (*http.Request, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for _, s := range specs {
		fw, err := w.CreateFormFile(fmt.Sprintf("file_%d", s.index), s.name)
		if err != nil {
			return nil, err
		}
		fw.Write(s.data)
		if s.vector != nil {
			b, _ := json.Marshal(s.vector)
			w.WriteField(fmt.Sprintf("vector_%d", s.index), string(b))
		} else {
			w.WriteField(fmt.Sprintf("vector_%d", s.index), "[]")
		}
		if len(s.preview) > 0 {
			pw, err := w.CreateFormFile(fmt.Sprintf("preview_%d", s.index), "preview.jpg")
			if err != nil {
				return nil, err
			}
			pw.Write(s.preview)
		}
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/upload/batch", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

// postBatch 直接调用 h.UploadBatch（同 upload_handler_integration_test.go 的直调风格）。
func postBatch(t *testing.T, h *Handler, specs []batchPartSpec) *httptest.ResponseRecorder {
	t.Helper()
	req, err := buildBatchRequest(specs)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.UploadBatch(c)
	return rec
}

type batchItemResp struct {
	Index        int    `json:"index"`
	Filename     string `json:"filename"`
	Status       string `json:"status"`
	ID           int64  `json:"id"`
	ExistingID   int64  `json:"existing_id"`
	FilePath     string `json:"file_path"`
	TakenAt      string `json:"taken_at"`
	Thumbnail    bool   `json:"thumbnail"`
	ThumbnailURL string `json:"thumbnail_url"`
	Duplicate    bool   `json:"duplicate"`
	Error        string `json:"error"`
}

type batchResp struct {
	Results []batchItemResp `json:"results"`
	Summary struct {
		Created   int   `json:"created"`
		Duplicate int   `json:"duplicate"`
		Failed    int   `json:"failed"`
		ElapsedMs int64 `json:"elapsed_ms"`
	} `json:"summary"`
}

func parseBatch(t *testing.T, rec *httptest.ResponseRecorder) batchResp {
	t.Helper()
	var got batchResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse batch response: %v body=%s", err, rec.Body.String())
	}
	return got
}

// setupUploadHandler 装一个用临时目录做落盘/缩略图的 handler。
func setupUploadHandler(t *testing.T) (*Handler, string, string) {
	t.Helper()
	h := setupHandler(t)
	up, th := t.TempDir(), t.TempDir()
	h.UploadSvc = service.NewUploadService(h.DB, nil, up, th)
	return h, up, th
}

// cleanupByToken 在请求之前注册自清理：按文件名 token 删掉本测试造成的所有行。
// 必须早于断言注册——同哈希并发时谁先拿到 advisory lock 不确定，
// 断言可能在任何一步失败，清理都必须照样发生。
func cleanupByToken(t *testing.T, h *Handler, token string) {
	t.Helper()
	t.Cleanup(func() {
		pattern := "%" + token + "%"
		h.DB.Exec("DELETE FROM photo_embeddings WHERE photo_id IN (SELECT id FROM photos WHERE file_path LIKE ?)", pattern)
		h.DB.Exec("DELETE FROM photos WHERE file_path LIKE ?", pattern)
	})
}

// ── 测试 ────────────────────────────────────────────────

// 一批里同时出现「新建 / 重复 / 坏向量」→ 200 + 逐项状态 + 部分成功。
func TestUploadBatchMixed(t *testing.T) {
	h, _, _ := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_mixed")
	img := uniqueJPEG(t, 640, 480, 101)
	other := uniqueJPEG(t, 640, 480, 202)

	rec := postBatch(t, h, []batchPartSpec{
		{index: 0, name: "uptest_mixed_a.jpg", data: img, vector: unitVector()},
		{index: 1, name: "uptest_mixed_a.jpg", data: img, vector: unitVector()}, // 同哈希 → 重复
		{index: 2, name: "uptest_mixed_c.jpg", data: other, vector: nil},        // 坏向量
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := parseBatch(t, rec)
	if len(got.Results) != 3 {
		t.Fatalf("want 3 results, got %d: %s", len(got.Results), rec.Body.String())
	}
	if got.Summary.Created != 1 || got.Summary.Duplicate != 1 || got.Summary.Failed != 1 {
		t.Fatalf("summary 错误: %+v (%s)", got.Summary, rec.Body.String())
	}

	var createdID, dupExisting int64
	for _, r := range got.Results {
		switch r.Status {
		case service.StatusCreated:
			createdID = r.ID
			if r.ID == 0 || r.TakenAt == "" || !strings.Contains(r.ThumbnailURL, "/api/v1/thumbnails/") {
				t.Fatalf("created 项字段不全: %+v", r)
			}
		case service.StatusDuplicate:
			dupExisting = r.ExistingID
			if !r.Duplicate {
				t.Fatalf("duplicate 标志缺失: %+v", r)
			}
		case service.StatusError:
			if r.Error != "invalid vector" {
				t.Fatalf("error 项原因应为 invalid vector: %+v", r)
			}
		default:
			t.Fatalf("未知状态: %+v", r)
		}
	}
	if createdID == 0 || dupExisting != createdID {
		t.Fatalf("重复项应指向同批新建的 id: created=%d dup_existing=%d (%s)", createdID, dupExisting, rec.Body.String())
	}

	// 落盘：恰好一行 DB 记录，且文件真的在
	var n int64
	h.DB.Raw("SELECT count(*) FROM photos WHERE file_hash = (SELECT file_hash FROM photos WHERE id = ?)", createdID).Scan(&n)
	if n != 1 {
		t.Fatalf("同哈希应有且只有一行，got %d", n)
	}
	var path string
	h.DB.Raw("SELECT file_path FROM photos WHERE id = ?", createdID).Scan(&path)
	if !strings.HasPrefix(path, "uploads/") {
		t.Fatalf("file_path 前缀错误: %s", path)
	}
}

// 带客户端预览 → 走快路径，缩略图长边必须是 1080。
func TestUploadBatchPreviewFastPath(t *testing.T) {
	h, _, th := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_prev")
	img := uniqueJPEG(t, 3000, 2000, 303)
	preview := uniqueJPEG(t, 1080, 720, 303) // 模拟客户端缩略图源

	rec := postBatch(t, h, []batchPartSpec{
		{index: 0, name: "uptest_prev.jpg", data: img, vector: unitVector(), preview: preview},
	})
	got := parseBatch(t, rec)
	if got.Results[0].Status != service.StatusCreated {
		t.Fatalf("应 created: %+v body=%s", got.Results[0], rec.Body.String())
	}
	if !got.Results[0].Thumbnail {
		t.Fatal("应生成缩略图")
	}
	w, hh := webpDims(t, filepath.Join(th, fmt.Sprintf("%d.webp", got.Results[0].ID)))
	if w != 1080 || hh != 720 {
		t.Fatalf("缩略图应为 1080x720，实际 %dx%d", w, hh)
	}
}

// 没有客户端预览时服务端兜底：长边缩到 1080（不是短边）。
func TestUploadBatchFallbackThumbnailLongSide(t *testing.T) {
	h, _, th := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_fallback")
	img := uniqueJPEG(t, 2400, 1600, 404)

	rec := postBatch(t, h, []batchPartSpec{
		{index: 0, name: "uptest_fallback.jpg", data: img, vector: unitVector()},
	})
	got := parseBatch(t, rec)
	if got.Results[0].Status != service.StatusCreated {
		t.Fatalf("应 created: %+v", got.Results[0])
	}
	w, hh := webpDims(t, filepath.Join(th, fmt.Sprintf("%d.webp", got.Results[0].ID)))
	if w != 1080 || hh != 720 {
		t.Fatalf("缩略图长边应为 1080（1080x720），实际 %dx%d", w, hh)
	}
}

// 回归：v1 的 PNG 上传必然生成不出缩略图（只注册了 image/jpeg）。
func TestUploadBatchPngThumbnail(t *testing.T) {
	h, _, th := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_png")
	data := uniquePNG(t, 1600, 1200, 505)

	rec := postBatch(t, h, []batchPartSpec{
		{index: 0, name: "uptest_png.png", data: data, vector: unitVector()},
	})
	got := parseBatch(t, rec)
	if got.Results[0].Status != service.StatusCreated {
		t.Fatalf("应 created: %+v", got.Results[0])
	}
	if !got.Results[0].Thumbnail {
		t.Fatalf("PNG 应能生成缩略图（v1 的已知 bug）: %+v", got.Results[0])
	}
	out := filepath.Join(th, fmt.Sprintf("%d.webp", got.Results[0].ID))
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("缩略图文件缺失: %v", err)
	}
	w, hh := webpDims(t, out)
	if w != 1080 || hh != 810 {
		t.Fatalf("PNG 缩略图应为 1080x810，实际 %dx%d", w, hh)
	}
}

// 同一份内容并发（同批 + 不同文件名）只允许落一行。
func TestUploadBatchConcurrentSameHash(t *testing.T) {
	h, _, _ := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_race")
	img := uniqueJPEG(t, 800, 600, 606)

	specs := make([]batchPartSpec, 0, 6)
	for i := 0; i < 6; i++ {
		specs = append(specs, batchPartSpec{
			index:  i,
			name:   fmt.Sprintf("uptest_race_%d.jpg", i), // 文件名不同 → 躲过快路径，真并发
			data:   img,
			vector: unitVector(),
		})
	}
	rec := postBatch(t, h, specs)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := parseBatch(t, rec)
	if got.Summary.Created != 1 || got.Summary.Duplicate != 5 {
		t.Fatalf("并发同哈希应 1 created + 5 duplicate，实际 %+v (%s)", got.Summary, rec.Body.String())
	}
	var id int64
	for _, r := range got.Results {
		if r.Status == service.StatusCreated {
			id = r.ID
		}
	}

	var n int64
	h.DB.Raw("SELECT count(*) FROM photos WHERE file_hash = (SELECT file_hash FROM photos WHERE id = ?)", id).Scan(&n)
	if n != 1 {
		t.Fatalf("库里应恰好一行，实际 %d", n)
	}
}

// 只有 vector 没有 file → 该项报错，其它项不受影响。
func TestUploadBatchMissingFilePart(t *testing.T) {
	h, _, _ := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_missing")
	img := uniqueJPEG(t, 640, 480, 707)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	b, _ := json.Marshal(unitVector())
	w.WriteField("vector_0", string(b)) // 没有 file_0
	fw, _ := w.CreateFormFile("file_1", "uptest_missing_ok.jpg")
	fw.Write(img)
	w.WriteField("vector_1", string(b))
	w.Close()

	rec := doBatch(t, h, body, w.FormDataContentType())

	got := parseBatch(t, rec)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200: %s", rec.Body.String())
	}
	if got.Results[0].Status != service.StatusError || got.Results[0].Error != "missing file" {
		t.Fatalf("index 0 应 missing file: %+v", got.Results[0])
	}
	if got.Results[1].Status != service.StatusCreated {
		t.Fatalf("index 1 应正常入库: %+v", got.Results[1])
	}
}

// doBatch 用给定的原始 body 调用批量接口。
func doBatch(t *testing.T, h *Handler, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/upload/batch", body)
	req.Header.Set("Content-Type", contentType)
	c.Request = req
	h.UploadBatch(c)
	return rec
}

func TestUploadBatchRejectsUnknownPart(t *testing.T) {
	h, _, _ := setupUploadHandler(t)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.WriteField("fileX", "nope")
	w.Close()

	rec := doBatch(t, h, body, w.FormDataContentType())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadBatchRejectsIndexBeyondLimit(t *testing.T) {
	h, _, _ := setupUploadHandler(t)
	img := uniqueJPEG(t, 320, 240, 808)
	rec := postBatch(t, h, []batchPartSpec{
		{index: 99, name: "too_big_index.jpg", data: img, vector: unitVector()},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadBatchEmptyBody(t *testing.T) {
	h, _, _ := setupUploadHandler(t)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.Close() // 只有结束边界，没有任何 part

	rec := doBatch(t, h, body, w.FormDataContentType())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 同一份内容被多个并发请求上传（跨请求竞争）也只允许落一行。
// 注意：goroutine 里不能用 t.Fatal，这里把错误收集起来在主 goroutine 断言。
func TestUploadBatchCrossRequestRace(t *testing.T) {
	h, up, _ := setupUploadHandler(t)
	cleanupByToken(t, h, "uptest_xreq")
	img := uniqueJPEG(t, 720, 540, 909)

	const n = 4
	reqs := make([]*http.Request, n)
	for i := 0; i < n; i++ {
		req, err := buildBatchRequest([]batchPartSpec{
			{index: 0, name: fmt.Sprintf("uptest_xreq_%d.jpg", i), data: img, vector: unitVector()},
		})
		if err != nil {
			t.Fatal(err)
		}
		reqs[i] = req
	}

	var wg sync.WaitGroup
	bodies := make([]string, n)
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = reqs[i]
			h.UploadBatch(c)
			codes[i] = rec.Code
			bodies[i] = rec.Body.String()
		}(i)
	}
	wg.Wait()

	created, duplicate := 0, 0
	var createdID int64
	for i := 0; i < n; i++ {
		if codes[i] != http.StatusOK {
			t.Fatalf("请求 %d 返回 %d: %s", i, codes[i], bodies[i])
		}
		var got batchResp
		if err := json.Unmarshal([]byte(bodies[i]), &got); err != nil {
			t.Fatalf("解析 %d 失败: %v (%s)", i, err, bodies[i])
		}
		switch got.Results[0].Status {
		case service.StatusCreated:
			created++
			createdID = got.Results[0].ID
		case service.StatusDuplicate:
			duplicate++
		}
	}
	if created != 1 || duplicate != n-1 {
		t.Fatalf("跨请求并发应 1 created + %d duplicate，实际 created=%d dup=%d", n-1, created, duplicate)
	}

	var count int64
	h.DB.Raw("SELECT count(*) FROM photos WHERE file_hash = (SELECT file_hash FROM photos WHERE id = ?)", createdID).Scan(&count)
	if count != 1 {
		t.Fatalf("库里应恰好一行，实际 %d", count)
	}

	// 输家的孤儿文件必须被清掉（不同文件名 → 不同落盘路径）
	entries, err := os.ReadDir(filepath.Join(up))
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	err = filepath.Walk(up, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && !strings.HasPrefix(info.Name(), ".upload-") {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Fatalf("上传目录应只剩 1 个文件，实际 %d（entries=%d）", files, len(entries))
	}
}

// ── webp 头部解析（用于断言缩略图尺寸） ───────────────────

func webpDims(t *testing.T, path string) (int, int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read webp: %v", err)
	}
	if len(data) < 30 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Fatalf("不是合法 webp: %s", path)
	}
	i := 12
	for i+8 <= len(data) {
		fourcc := string(data[i : i+4])
		size := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		body := data[i+8:]
		switch fourcc {
		case "VP8 ":
			if len(body) < 10 {
				t.Fatal("VP8 头太短")
			}
			w := int(binary.LittleEndian.Uint16(body[6:8]) & 0x3fff)
			h := int(binary.LittleEndian.Uint16(body[8:10]) & 0x3fff)
			return w, h
		case "VP8L":
			if len(body) < 5 {
				t.Fatal("VP8L 头太短")
			}
			bits := uint32(body[1]) | uint32(body[2])<<8 | uint32(body[3])<<16 | uint32(body[4])<<24
			return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1
		case "VP8X":
			if len(body) < 18 {
				t.Fatal("VP8X 头太短")
			}
			w := int(uint32(body[4]) | uint32(body[5])<<8 | uint32(body[6])<<16)
			h := int(uint32(body[7]) | uint32(body[8])<<8 | uint32(body[9])<<16)
			return w + 1, h + 1
		}
		i += 8 + size + (size & 1)
	}
	t.Fatalf("webp 里找不到图像头: %s", path)
	return 0, 0
}
