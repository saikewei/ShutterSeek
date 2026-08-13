package service

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var unsafeChars = regexp.MustCompile(`[^0-9A-Za-z._-]`)

// sanitizeBaseName 净化原始文件名：取 basename、去路径分隔符/控制字符、截断 80 字符。
func sanitizeBaseName(name string) string {
	name = filepath.Base(name)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	base = unsafeChars.ReplaceAllString(base, "_")
	if len(base) > 80 {
		base = base[:80]
	}
	return base + ext
}

// buildUploadRelPath 生成 DB 相对路径 uploads/{YYYY}/{MM}/{YYYYMMDD-HHMMSS}_{原名}{_hash8}{ext}。
func buildUploadRelPath(takenAt time.Time, origName, hash8 string) string {
	base := sanitizeBaseName(origName)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	stamp := takenAt.Format("20060102-150405")
	name := stamp + "_" + stem
	if hash8 != "" {
		name += "_" + hash8
	}
	return filepath.Join("uploads", takenAt.Format("2006"), takenAt.Format("01"), name+ext)
}
