package service

import (
	"bytes"
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
	m, err := extractEXIFMany([]string{path})
	if err != nil {
		return nil, err
	}
	ex := m[path]
	if ex == nil {
		if len(m) == 1 {
			for _, only := range m {
				return only, nil
			}
		}
		return nil, fmt.Errorf("exiftool: no record for %s", path)
	}
	return ex, nil
}

// extractEXIFMany 一次 exiftool 调用提取多张照片元数据，按原路径索引。
//
// 批量上传时逐张 spawn exiftool 是纯浪费（实测单文件 176ms、四文件 246ms），
// 因此整批一次调用。exiftool 的汇总行走 stderr，stdout 始终是纯 JSON 数组；
// 这里不依赖退出码，只要 stdout 能解析出记录就算成功（个别文件告警不影响整批）。
func extractEXIFMany(paths []string) (map[string]*PhotoEXIF, error) {
	if len(paths) == 0 {
		return map[string]*PhotoEXIF{}, nil
	}
	args := append([]string{"-json"}, paths...)
	cmd := exec.Command("exiftool", args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	runErr := cmd.Run()

	var records []map[string]any
	if err := json.Unmarshal(out.Bytes(), &records); err != nil || len(records) == 0 {
		if runErr != nil {
			return nil, fmt.Errorf("exiftool: %w: %s", runErr, strings.TrimSpace(errb.String()))
		}
		return nil, fmt.Errorf("parse exiftool output")
	}
	return parseEXIFMany(records, func(src string) time.Time {
		if fi, err := os.Stat(src); err == nil {
			return fi.ModTime()
		}
		return time.Now()
	}), nil
}

// parseEXIFMany 纯函数：exiftool 的 JSON 记录数组 → 按 SourceFile 索引的元数据。
// fallbackOf 给出没有拍摄时间时使用的兜底时间（生产传文件的 ModTime）。
func parseEXIFMany(records []map[string]any, fallbackOf func(path string) time.Time) map[string]*PhotoEXIF {
	res := make(map[string]*PhotoEXIF, len(records))
	for _, r := range records {
		src, _ := r["SourceFile"].(string)
		if src == "" {
			continue
		}
		fallback := time.Now()
		if fallbackOf != nil {
			fallback = fallbackOf(src)
		}
		res[src] = parseEXIF(r, fallback)
	}
	return res
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
