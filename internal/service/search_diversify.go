package service

import (
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBurstWindow = 8 * time.Second
	defaultMMRLambda   = 0.75
)

type scoredCandidate struct {
	item   SearchItem
	vector []float32
}

// parseVectorLiteral parses a pgvector/halfvec text literal like "[0.1,-0.2]"
// into an L2-normalized float32 slice for cosine similarity comparisons.
func parseVectorLiteral(raw string) []float32 {
	s := strings.TrimSpace(raw)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil
	}
	body := strings.TrimSpace(s[1 : len(s)-1])
	if body == "" {
		return nil
	}
	parts := strings.Split(body, ",")
	vec := make([]float32, 0, len(parts))
	var sumSq float64
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		vec = append(vec, float32(f))
		sumSq += f * f
	}
	if sumSq <= 0 {
		return nil
	}
	invNorm := float32(1.0 / math.Sqrt(sumSq))
	for i := range vec {
		vec[i] *= invNorm
	}
	return vec
}

func cosineDot(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	if sum > 1 {
		return 1
	}
	if sum < -1 {
		return -1
	}
	return sum
}

// diversifyCandidates collapses close-in-time burst shots and applies MMR
// reranking so a burst does not monopolize the top results. Collapsed shots
// are deferred rather than dropped: if fewer than limit unique shots exist,
// deferred shots backfill the result so callers never get starved.
func diversifyCandidates(candidates []scoredCandidate, limit int, burstWindow time.Duration) []SearchItem {
	if limit <= 0 || len(candidates) == 0 {
		return []SearchItem{}
	}
	if len(candidates) == 1 {
		return []SearchItem{candidates[0].item}
	}

	// Stage 1: collapse burst duplicates while preserving highest-scoring shot.
	primary := make([]scoredCandidate, 0, len(candidates))
	deferred := make([]scoredCandidate, 0)
	for _, cand := range candidates {
		isBurst := false
		if burstWindow > 0 && !cand.item.TakenAt.IsZero() {
			for _, kept := range primary {
				if kept.item.TakenAt.IsZero() {
					continue
				}
				diff := cand.item.TakenAt.Sub(kept.item.TakenAt)
				if diff < 0 {
					diff = -diff
				}
				if diff <= burstWindow {
					isBurst = true
					break
				}
			}
		}
		if isBurst {
			deferred = append(deferred, cand)
		} else {
			primary = append(primary, cand)
		}
	}

	// Stage 2: greedy MMR selection over primary candidates.
	target := limit
	if len(primary) < target {
		target = len(primary)
	}
	selected := make([]scoredCandidate, 0, limit)
	used := make([]bool, len(primary))
	if target > 0 {
		selected = append(selected, primary[0])
		used[0] = true
	}
	for len(selected) < target {
		bestIdx := -1
		bestVal := -math.MaxFloat64
		for i, cand := range primary {
			if used[i] {
				continue
			}
			maxSim := 0.0
			if len(cand.vector) > 0 {
				for _, chosen := range selected {
					if sim := cosineDot(cand.vector, chosen.vector); sim > maxSim {
						maxSim = sim
					}
				}
			}
			mmr := defaultMMRLambda*cand.item.Score - (1.0-defaultMMRLambda)*maxSim
			if bestIdx == -1 || mmr > bestVal ||
				(mmr == bestVal && (cand.item.Score > primary[bestIdx].item.Score ||
					(cand.item.Score == primary[bestIdx].item.Score && cand.item.ID < primary[bestIdx].item.ID))) {
				bestIdx = i
				bestVal = mmr
			}
		}
		if bestIdx < 0 {
			break
		}
		used[bestIdx] = true
		selected = append(selected, primary[bestIdx])
	}

	// Stage 3: backfill from deferred burst shots if primary pool < limit.
	for i := 0; i < len(deferred) && len(selected) < limit; i++ {
		selected = append(selected, deferred[i])
	}

	out := make([]SearchItem, len(selected))
	for i := range selected {
		out[i] = selected[i].item
	}
	return out
}
