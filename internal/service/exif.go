package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type PhotoEXIF struct {
	TakenAt     *time.Time
	Width       int
	Height      int
	CameraMake  string
	CameraModel string
	LensModel   string
	FocalLength float64
	Aperture    float64
	ISO         int
}

// extractEXIF 调用 exiftool -json 提取单张照片元数据。
func extractEXIF(path string) (*PhotoEXIF, error) {
	out, err := exec.Command("exiftool", "-json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("exiftool: %w", err)
	}
	var records []map[string]any
	if err := json.Unmarshal(out, &records); err != nil || len(records) == 0 {
		return nil, fmt.Errorf("parse exiftool output")
	}
	fi, _ := os.Stat(path)
	fallback := time.Now()
	if fi != nil {
		fallback = fi.ModTime()
	}
	return parseEXIF(records[0], fallback), nil
}

// parseEXIF 纯函数：exiftool JSON → PhotoEXIF。测试用。
func parseEXIF(m map[string]any, fallback time.Time) *PhotoEXIF {
	ex := &PhotoEXIF{}
	if w, ok := toInt(m["ImageWidth"]); ok {
		ex.Width = w
	}
	if h, ok := toInt(m["ImageHeight"]); ok {
		ex.Height = h
	}
	ex.CameraMake, _ = m["Make"].(string)
	ex.CameraModel, _ = m["Model"].(string)
	ex.LensModel, _ = m["LensModel"].(string)
	if f, ok := m["FNumber"].(float64); ok {
		ex.Aperture = f
	}
	if f, ok := m["ISO"].(float64); ok {
		ex.ISO = int(f)
	}
	if s, ok := m["FocalLength"].(string); ok {
		fields := strings.Fields(s)
		if len(fields) > 0 {
			if f, err := strconv.ParseFloat(fields[0], 64); err == nil {
				ex.FocalLength = f
			}
		}
	}
	for _, key := range []string{"DateTimeOriginal", "CreateDate"} {
		if s, ok := m[key].(string); ok {
			if t, err := time.ParseInLocation("2006:01:02 15:04:05", s, cstZone); err == nil {
				ex.TakenAt = &t
				break
			}
		}
	}
	if ex.TakenAt == nil {
		t := fallback
		ex.TakenAt = &t
	}
	return ex
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i, true
		}
	}
	return 0, false
}
