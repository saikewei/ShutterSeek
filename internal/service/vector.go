package service

import (
	"encoding/json"
	"errors"
	"math"
)

var ErrInvalidVector = errors.New("invalid vector")

// ParseVector 校验客户端上传的 1024 维向量：JSON 数组、长度 1024、
// 无 NaN/Inf、L2 范数 ∈ [0.9, 1.1]（容忍 fp16 舍入）。
func ParseVector(s string) ([]float32, error) {
	var v []float32
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, ErrInvalidVector
	}
	if len(v) != 1024 {
		return nil, ErrInvalidVector
	}
	var sum float64
	for _, f := range v {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return nil, ErrInvalidVector
		}
		sum += float64(f) * float64(f)
	}
	norm := math.Sqrt(sum)
	if norm < 0.9 || norm > 1.1 {
		return nil, ErrInvalidVector
	}
	return v, nil
}
