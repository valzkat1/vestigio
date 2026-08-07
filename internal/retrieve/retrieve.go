// Package retrieve holds the ranking layer.
//
// In v1 there is exactly one scorer (BM25 over FTS5), so fusion is a no-op that
// preserves BM25 order. The interface and the fusion step exist anyway, and that
// is the entire point: turning on embeddings later means registering a second
// Scorer and backfilling vectors — not reworking retrieval.
package retrieve

import "sort"

// Hit is one scorer's opinion about one memory. Rank is 0-based and is what
// fusion consumes; Score is kept for debugging and is not comparable across
// scorers, which is precisely why RRF ignores it.
type Hit struct {
	ID    int64
	Rank  int
	Score float64
}

// Scorer produces a ranked candidate list. A future EmbeddingScorer satisfies
// this interface without touching anything else.
type Scorer interface {
	Name() string
	Search(project, query string, limit int) ([]Hit, error)
}

// rrfK dampens the influence of top ranks. 60 is the value from the original
// reciprocal-rank-fusion work and is the usual default.
const rrfK = 60.0

// Fuse merges ranked lists by Reciprocal Rank Fusion: score = Σ 1/(k + rank).
//
// RRF uses only positions, never raw scores, so a BM25 score and a cosine
// similarity can be combined without normalising either one.
func Fuse(lists ...[]Hit) []int64 {
	agg := map[int64]float64{}
	for _, list := range lists {
		for _, h := range list {
			agg[h.ID] += 1.0 / (rrfK + float64(h.Rank))
		}
	}
	ids := make([]int64, 0, len(agg))
	for id := range agg {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if agg[ids[i]] != agg[ids[j]] {
			return agg[ids[i]] > agg[ids[j]]
		}
		return ids[i] < ids[j] // stable output for equal scores
	})
	return ids
}
