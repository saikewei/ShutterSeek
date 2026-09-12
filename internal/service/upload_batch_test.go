package service

import "testing"

func TestSummarizeUpload(t *testing.T) {
	results := []UploadResult{
		{Index: 0, Status: StatusCreated, ThumbMs: 100},
		{Index: 1, Status: StatusDuplicate},
		{Index: 2, Status: StatusError, Err: "invalid vector"},
		{Index: 3, Status: StatusCreated, ThumbMs: 50},
	}
	sum := summarizeUpload(results)
	if sum.Created != 2 || sum.Duplicate != 1 || sum.Failed != 1 {
		t.Fatalf("计数错误: %+v", sum)
	}
	if sum.SumThumbMs != 150 {
		t.Fatalf("缩略图总耗时错误: %d", sum.SumThumbMs)
	}
}

func TestSummarizeUploadEmpty(t *testing.T) {
	sum := summarizeUpload(nil)
	if sum.Created != 0 || sum.Duplicate != 0 || sum.Failed != 0 {
		t.Fatalf("空结果应为全零: %+v", sum)
	}
}

func TestUploadOptionsNormalized(t *testing.T) {
	got := UploadOptions{}.normalized()
	if got.Workers != DefaultUploadWorkers || got.BatchMaxFiles != DefaultBatchMaxFiles || got.MaxBatchBytes != DefaultMaxBatchBytes {
		t.Fatalf("零值应归一化成默认: %+v", got)
	}
	got = UploadOptions{Workers: 3, BatchMaxFiles: 2, MaxBatchBytes: 1024}.normalized()
	if got.Workers != 3 || got.BatchMaxFiles != 2 || got.MaxBatchBytes != 1024 {
		t.Fatalf("合法值不应被改写: %+v", got)
	}
}
