package retrieve

import (
	"fmt"
	"reflect"
	"testing"
)

// Read this before trusting the coverage number for this package.
//
// As of these tests, nothing imports internal/retrieve: Fuse has zero call
// sites and Scorer has zero implementations. The ranking that actually runs is
// `ORDER BY bm25(memories_fts)` inside store.search — SQLite does it.
//
// So these tests lock a contract for the day M2 wires a second scorer in. They
// are not evidence that recall works, and this package reaching 100% must not
// be read as the retrieval layer being covered.

// list is a ranked candidate list in the order a scorer would emit it. Score is
// filled with a value that would sort the OPPOSITE way if anyone ever read it,
// so a regression that starts consulting Score fails loudly.
func list(ids ...int64) []Hit {
	hits := make([]Hit, len(ids))
	for i, id := range ids {
		hits[i] = Hit{ID: id, Rank: i, Score: float64(i)}
	}
	return hits
}

// The v1 invariant: one scorer means fusion must be a no-op. If this breaks,
// every recall in production silently reorders.
func TestFuseWithOneListPreservesRankOrder(t *testing.T) {
	in := list(42, 7, 99, 1)
	want := []int64{42, 7, 99, 1}
	if got := Fuse(in); !reflect.DeepEqual(got, want) {
		t.Errorf("Fuse(single list) = %v, want %v — with one scorer, fusion must not reorder", got, want)
	}
}

func TestFuseWithOneLongListPreservesRankOrder(t *testing.T) {
	ids := make([]int64, 100)
	for i := range ids {
		ids[i] = int64(1000 - i) // descending ids: order can only come from Rank
	}
	if got := Fuse(list(ids...)); !reflect.DeepEqual(got, ids) {
		t.Errorf("Fuse reordered a 100-item list; first divergence in %v", got[:5])
	}
}

func TestFuseEmptyInputs(t *testing.T) {
	cases := map[string][][]Hit{
		"no lists at all":  nil,
		"one empty list":   {{}},
		"several empty":    {{}, {}, {}},
		"empty plus empty": {nil, nil},
	}
	for name, lists := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Fuse(lists...); len(got) != 0 {
				t.Errorf("Fuse(%s) = %v, want empty", name, got)
			}
		})
	}
}

func TestFuseIgnoresEmptyListsAmongRealOnes(t *testing.T) {
	want := []int64{5, 6}
	if got := Fuse(nil, list(5, 6), nil); !reflect.DeepEqual(got, want) {
		t.Errorf("Fuse = %v, want %v — an empty scorer must not disturb a working one", got, want)
	}
}

// Agreement across scorers is the whole point of RRF: a memory both scorers
// like beats a memory only one of them ranked first.
func TestFuseRewardsAgreementOverASingleTopRank(t *testing.T) {
	bm25 := list(1, 2)   // 1 is best here
	vector := list(3, 2) // 3 is best here, 2 is second in both
	got := Fuse(bm25, vector)

	// 2 → 1/61 + 1/61 = 0.0328   ·   1 and 3 → 1/60 = 0.0167 each
	if got[0] != 2 {
		t.Errorf("Fuse = %v, want id 2 first: ranked second by BOTH scorers beats first by one", got)
	}
}

// RRF combines positions, never raw scores — that is what lets a BM25 score and
// a cosine similarity be merged without normalising either one.
func TestFuseIgnoresRawScores(t *testing.T) {
	plain := []Hit{{ID: 10, Rank: 0, Score: 0.01}, {ID: 20, Rank: 1, Score: 0.02}}
	wild := []Hit{{ID: 10, Rank: 0, Score: -9999}, {ID: 20, Rank: 1, Score: 9999}}

	if a, b := Fuse(plain), Fuse(wild); !reflect.DeepEqual(a, b) {
		t.Errorf("Fuse(plain) = %v but Fuse(wild scores) = %v — Score must not influence fusion", a, b)
	}
}

// Map iteration in Go is deliberately randomised, so equal scores without an
// explicit tiebreak would shuffle results between calls on identical input.
func TestFuseIsDeterministicOnTies(t *testing.T) {
	a, b := list(1, 2), list(2, 1) // perfectly symmetric: both ids score 1/60 + 1/61
	want := []int64{1, 2}          // tie broken by ascending id

	for i := 0; i < 200; i++ {
		if got := Fuse(a, b); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: Fuse = %v, want %v — ties must break by id, not by map order", i, got, want)
		}
	}
}

func TestFuseIsDeterministicAcrossManyTiedIDs(t *testing.T) {
	var a, b []Hit
	for i := int64(1); i <= 20; i++ {
		a = append(a, Hit{ID: i, Rank: 0}) // every id tied at rank 0
		b = append(b, Hit{ID: i, Rank: 0})
	}
	first := Fuse(a, b)
	for i := 0; i < 50; i++ {
		if got := Fuse(a, b); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d produced %v, first run produced %v", i, got, first)
		}
	}
	for i, id := range first {
		if id != int64(i+1) {
			t.Fatalf("tied ids must come back ascending, got %v", first)
		}
	}
}

func TestFuseUnionsIDsAcrossLists(t *testing.T) {
	got := Fuse(list(1, 2), list(3, 4), list(5))
	if len(got) != 5 {
		t.Errorf("Fuse = %v, want all 5 ids: fusion is a union, not an intersection", got)
	}
}

// The k in 1/(k+rank) damps how much the top of one list can dominate. With
// k=60, ranks 0 and 1 are nearly equal — that is deliberate, and it is why
// agreement wins over a single confident scorer.
func TestRRFDampeningIsGentleAtTheTop(t *testing.T) {
	if rrfK != 60.0 {
		t.Fatalf("rrfK = %v, want 60 (the value from the original RRF paper)", rrfK)
	}
	top, second := 1.0/(rrfK+0), 1.0/(rrfK+1)
	if ratio := top / second; ratio > 1.02 {
		t.Errorf("rank 0 outweighs rank 1 by %.3fx; with k=60 it should be ~1.017x", ratio)
	}
	// One list ranking X first cannot beat two lists ranking Y second.
	if got := Fuse(list(1, 9), list(2, 9)); got[0] != 9 {
		t.Errorf("Fuse = %v, want 9 first: two second-places must outweigh one first-place", got)
	}
}

func TestFuseHandlesSparseAndDeepRanks(t *testing.T) {
	deep := []Hit{{ID: 1, Rank: 500}}
	shallow := []Hit{{ID: 2, Rank: 0}}
	if got := Fuse(deep, shallow); !reflect.DeepEqual(got, []int64{2, 1}) {
		t.Errorf("Fuse = %v, want [2 1]: rank 500 must not outrank rank 0", got)
	}
}

func ExampleFuse() {
	bm25 := []Hit{{ID: 7, Rank: 0}, {ID: 4, Rank: 1}}
	vector := []Hit{{ID: 4, Rank: 0}, {ID: 9, Rank: 1}}
	fmt.Println(Fuse(bm25, vector))
	// Output: [4 7 9]
}
