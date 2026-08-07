// Package importer migrates an Engram JSON export into vestigio.
//
// Migration is not a copy. An export carries three problems that a straight
// insert would preserve forever:
//
//  1. Project names drift. The same repository shows up as "alcubo",
//     "alcubo-backend" and "reposa2censo" depending on how detection resolved
//     that day, and memory split across name variants is memory that never gets
//     recalled.
//  2. Types do not line up. Engram has eleven; vestigio has five, deliberately.
//  3. Memories are large. In the corpus this was written against the median
//     memory was ~1,574 characters — with an 800-token budget, two of them fill
//     the whole response.
//
// Bodies are still imported whole. Truncating on the way in would be lossy and
// irreversible, and the full text is what a future embedding backfill needs.
// Fitting oversized memories into a budget is a read-time problem, and belongs
// to the packer.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/valzkat1/vestigio/internal/store"
)

type Export struct {
	Version      string        `json:"version"`
	Observations []Observation `json:"observations"`
}

type Observation struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Project   string `json:"project"`
	Scope     string `json:"scope"`
	TopicKey  string `json:"topic_key"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// kindMap folds Engram's eleven types into vestigio's five.
//
// The collapses are judgement calls worth stating: an architecture note is a
// decision with more prose around it, and a preference or a piece of feedback is
// a constraint on how work gets done. Anything unmapped becomes a reference,
// which is the honest default for "worth keeping, no stronger claim".
var kindMap = map[string]string{
	"decision":     "decision",
	"architecture": "decision",
	"bugfix":       "bugfix",
	"pattern":      "pattern",
	"preference":   "constraint",
	"feedback":     "constraint",
	"config":       "constraint",
	"discovery":    "reference",
	"analysis":     "reference",
	"manual":       "reference",
}

type Options struct {
	ProjectMap map[string]string // canonical project names
	SkipTypes  map[string]bool   // Engram types to drop entirely
	DryRun     bool
}

type Result struct {
	Total      int
	Imported   int
	Duplicates int
	Skipped    map[string]int // by Engram type
	ByProject  map[string]int
	ByKind     map[string]int
	Chars      int
	Oversized  int // memories larger than a default 800-token budget on their own
	Errors     []string
}

const defaultBudgetTokens = 800

func Load(path string) (*Export, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	var e Export
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("parse export: %w", err)
	}
	return &e, nil
}

func Run(st *store.Store, e *Export, opt Options) *Result {
	r := &Result{
		Skipped:   map[string]int{},
		ByProject: map[string]int{},
		ByKind:    map[string]int{},
	}

	for _, o := range e.Observations {
		r.Total++

		if opt.SkipTypes[o.Type] {
			r.Skipped[o.Type]++
			continue
		}
		if strings.TrimSpace(o.Title) == "" || strings.TrimSpace(o.Content) == "" {
			r.Skipped["(vacío)"]++
			continue
		}

		project := canonicalProject(o, opt.ProjectMap)
		kind := kindMap[o.Type]
		if kind == "" {
			kind = "reference"
		}

		r.ByProject[project]++
		r.ByKind[kind]++
		r.Chars += len(o.Content)
		if store.EstimateTokens(o.Content) > defaultBudgetTokens {
			r.Oversized++
		}

		if opt.DryRun {
			r.Imported++
			continue
		}

		created, updated := parseTime(o.CreatedAt), parseTime(o.UpdatedAt)
		_, isNew, err := st.Import(project, kind, o.Title, o.Content, created, updated)
		switch {
		case err != nil:
			if len(r.Errors) < 10 {
				r.Errors = append(r.Errors, fmt.Sprintf("#%d %q: %v", o.ID, o.Title, err))
			}
		case isNew:
			r.Imported++
		default:
			r.Duplicates++
		}
	}
	return r
}

// canonicalProject applies the rename map. Personal-scope memories are pulled
// out of whatever project they were captured in — they follow the person, not
// the repository.
func canonicalProject(o Observation, m map[string]string) string {
	if o.Scope == "personal" {
		return "personal"
	}
	p := strings.ToLower(strings.TrimSpace(o.Project))
	if p == "" {
		return "default"
	}
	if to, ok := m[p]; ok {
		return to
	}
	return p
}

// parseTime accepts the formats an export can carry and falls back to now,
// because a memory with no date is still worth more than no memory.
func parseTime(s string) int64 {
	if s == "" {
		return time.Now().Unix()
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Unix()
}

func (r *Result) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "total %d · importadas %d · duplicadas %d\n", r.Total, r.Imported, r.Duplicates)

	if len(r.Skipped) > 0 {
		fmt.Fprintf(&b, "\nomitidas:\n")
		for _, kv := range sortedDesc(r.Skipped) {
			fmt.Fprintf(&b, "  %4d  %s\n", kv.v, kv.k)
		}
	}
	fmt.Fprintf(&b, "\npor proyecto:\n")
	for _, kv := range sortedDesc(r.ByProject) {
		fmt.Fprintf(&b, "  %4d  %s\n", kv.v, kv.k)
	}
	fmt.Fprintf(&b, "\npor kind:\n")
	for _, kv := range sortedDesc(r.ByKind) {
		fmt.Fprintf(&b, "  %4d  %s\n", kv.v, kv.k)
	}

	avg := 0
	if r.Imported > 0 {
		avg = r.Chars / r.Imported
	}
	fmt.Fprintf(&b, "\ntamaño: %d chars (~%d tok) · promedio %d chars (~%d tok)\n",
		r.Chars, store.EstimateTokens(strings.Repeat("x", r.Chars)), avg, avg/4)
	fmt.Fprintf(&b, "memorias que solas exceden un budget de %d tok: %d\n", defaultBudgetTokens, r.Oversized)

	for _, e := range r.Errors {
		fmt.Fprintf(&b, "  ERROR %s\n", e)
	}
	return b.String()
}

type kv struct {
	k string
	v int
}

func sortedDesc(m map[string]int) []kv {
	out := make([]kv, 0, len(m))
	for k, v := range m {
		out = append(out, kv{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].v != out[j].v {
			return out[i].v > out[j].v
		}
		return out[i].k < out[j].k
	})
	return out
}
