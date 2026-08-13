//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

// 上传一张临时小图 + 合法单位向量 → 201；再传同图 → 409。
func TestUploadDuplicate(t *testing.T) {
	h := setupHandler(t)
	h.UploadSvc = service.NewUploadService(h.DB, nil, t.TempDir(), t.TempDir())
	gin.SetMode(gin.TestMode)

	img := makeTestJPEG(t)
	vec := unitVector()

	post := func() *httptest.ResponseRecorder {
		body := &bytes.Buffer{}
		w := multipart.NewWriter(body)
		fw, _ := w.CreateFormFile("file", "test_upload.jpg")
		fw.Write(img)
		w.WriteField("vector", string(mustJSON(t, vec)))
		w.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/upload", body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = req
		h.Upload(c)
		return rec
	}

	rec := post()
	if rec.Code != http.StatusCreated {
		t.Fatalf("first upload: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("parse created: %v body=%s", err, rec.Body.String())
	}
	t.Cleanup(func() {
		h.DB.Exec("DELETE FROM photo_embeddings WHERE photo_id = ?", created.ID)
		h.DB.Exec("DELETE FROM photos WHERE id = ?", created.ID)
	})

	rec2 := post()
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second upload: %d %s", rec2.Code, rec2.Body.String())
	}
	var dup struct {
		Duplicate  bool  `json:"duplicate"`
		ExistingID int64 `json:"existing_id"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &dup); err != nil {
		t.Fatalf("parse duplicate: %v", err)
	}
	if !dup.Duplicate || dup.ExistingID != created.ID {
		t.Fatalf("duplicate mismatch: %+v", dup)
	}
}

func makeTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < 32*32; i++ {
		img.Set(i%32, i/32, color.RGBA{R: uint8(i), G: uint8(255 - i), B: 128, A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func unitVector() []float32 {
	v := make([]float32, 1024)
	for i := range v {
		v[i] = 0.03125 // 1/32 → L2 范数 = 1
	}
	return v
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
