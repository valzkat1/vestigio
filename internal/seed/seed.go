// Package seed turns a document into memories.
//
// The hard part is not reading the file, it is CUTTING it. A memory that needs
// more than a few hundred tokens to state is two memories — and a README pasted
// in whole is one row that no sensible budget_tokens can ever return. The M0
// Engram import already produced 13 memories that individually blow an 800-token
// budget; this package exists so seeding does not add more.
//
// Parsing is pure: no store, no filesystem, no network. The CLI does the I/O.
package seed

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/valzkat1/vestigio/internal/store"
)

// DefaultMaxTokens is the size above which a section is split if it can be.
//
// 400 is half the default recall budget, so two memories of this size still fit
// one answer. It is a heuristic about usefulness, not a storage limit.
const DefaultMaxTokens = 400

// Memory is one parsed fact, ready for store.Remember.
type Memory struct {
	Title  string
	Body   string
	Kind   string
	Tokens int

	// Split reports that this memory came from cutting an oversized section
	// rather than from a heading the author wrote. Surfaced in the dry-run:
	// auto-splitting invents titles, and the operator should see which ones.
	Split bool

	// Oversized reports a section over MaxTokens that had no sub-headings to
	// split on. It is stored anyway, because dropping the user's content is
	// worse than storing something awkward — but it is reported.
	Oversized bool
}

// Options tune the cut. The zero value is valid and means "auto".
type Options struct {
	Kind       string // fallback when the cascade finds nothing
	SplitLevel int    // heading level to cut at; 0 = auto-detect
	MaxTokens  int    // 0 = DefaultMaxTokens
}

// Result carries what to store plus what the operator should know before it is.
type Result struct {
	Memories   []Memory
	SplitLevel int // the level actually used, so --dry-run can report the guess
	Skipped    int // headings with no content: a title alone is not a fact
}

// Parse converts a document into memories. Format is chosen by extension:
// markdown cuts on headings, anything else cuts on `---` lines.
func Parse(name, content string, opt Options) (Result, error) {
	if opt.MaxTokens <= 0 {
		opt.MaxTokens = DefaultMaxTokens
	}
	if opt.Kind != "" && !store.Kinds[opt.Kind] {
		return Result{}, fmt.Errorf("unknown kind %q", opt.Kind)
	}
	if opt.SplitLevel < 0 || opt.SplitLevel > 6 {
		return Result{}, fmt.Errorf("--split must be between 1 and 6, got %d", opt.SplitLevel)
	}
	if isMarkdown(name) {
		return parseMarkdown(content, opt)
	}
	return parseDelimited(content, opt)
}

func isMarkdown(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// ---------------------------------------------------------------- markdown

type node struct {
	level int // 1..6; 0 is the preamble before any heading
	title string
	lead  []string // lines directly under this heading, before any child
	kids  []*node
	kind  string // resolved from the heading text, "" if it names no kind
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	markerRe  = regexp.MustCompile(`^\[([A-Za-z]+)\]\s*(.*)$`)
	fenceRe   = regexp.MustCompile("^\\s*(```|~~~)")
)

func parseMarkdown(content string, opt Options) (Result, error) {
	root := &node{level: 0}
	stack := []*node{root}
	counts := map[int]int{} // headings seen per level
	labels := map[int]int{} // of those, how many merely name a kind
	inFence := false

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")

		// A `#` inside a fenced block is a shell comment, not a heading. Docs
		// are full of them, and treating one as a section is how a code sample
		// becomes three memories.
		if fenceRe.MatchString(line) {
			inFence = !inFence
			appendLead(stack, line)
			continue
		}
		if inFence {
			appendLead(stack, line)
			continue
		}

		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			appendLead(stack, line)
			continue
		}

		level := len(m[1])
		kind, title := splitMarker(strings.TrimSpace(m[2]))
		counts[level]++
		if kind == "" {
			// Only a heading that IS a kind name counts as a label. An explicit
			// `[decision]` marker labels a real fact, so it must not.
			if fromTitle := kindFromTitle(title); fromTitle != "" {
				kind = fromTitle
				labels[level]++
			}
		}

		n := &node{level: level, title: title, kind: kind}
		for len(stack) > 1 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		parent.kids = append(parent.kids, n)
		stack = append(stack, n)
	}

	level := opt.SplitLevel
	if level == 0 {
		level = detectLevel(counts, labels)
	}

	res := Result{SplitLevel: level}
	if level == 0 {
		// No headings at all. One document, one memory — and almost certainly
		// oversized, which the caller will see and can act on.
		body := strings.TrimSpace(strings.Join(root.lead, "\n"))
		if body == "" {
			return res, nil
		}
		title, rest := firstLineAsTitle(body)
		res.Memories = append(res.Memories, mkMemory(title, rest, fallbackKind("", opt), false, opt))
		return res, nil
	}

	collect(root, level, nil, opt, &res)
	return res, nil
}

func appendLead(stack []*node, line string) {
	n := stack[len(stack)-1]
	n.lead = append(n.lead, line)
}

// detectLevel picks the shallowest heading level that appears more than once
// and is not a layer of category labels.
//
// A document listing many facts repeats one level — `## A`, `## B`, `## C` under
// a single `# Title`. A file holding several whole records repeats `#`. Cutting
// at the shallowest REPEATED level gets both right.
//
// The label rule is the part that had to be learned by running it. A file
// organised as `# Decisiones` / `# Restricciones`, each with several `##` facts
// under it, repeats at level 1 — so the naive rule cut there and produced two
// enormous memories named after the categories. But a heading that IS a kind
// name exists to CLASSIFY its children, which is exactly what the kind cascade
// reads it for. A level whose headings all name kinds is a labelling layer, and
// cutting on it destroys the cut and the cascade at once. So such levels are
// skipped.
//
// Still not solved, deliberately: a file with a single record (`# ADR-001` then
// `## Context`, `## Decision`) has no repeat at level 1 and cuts at 2, splitting
// one record into its parts. Nothing in the text distinguishes that from a list
// of two facts. `--split=1` is the answer and `--dry-run` is how it gets seen.
func detectLevel(counts, labels map[int]int) int {
	isLabelLayer := func(lv int) bool { return counts[lv] > 0 && labels[lv] == counts[lv] }

	for lv := 1; lv <= 6; lv++ {
		if counts[lv] > 1 && !isLabelLayer(lv) {
			return lv
		}
	}
	for lv := 1; lv <= 6; lv++ {
		if counts[lv] > 0 && !isLabelLayer(lv) {
			return lv
		}
	}
	// Every heading in the document names a kind. Unusual, but cutting
	// somewhere beats returning nothing.
	for lv := 1; lv <= 6; lv++ {
		if counts[lv] > 0 {
			return lv
		}
	}
	return 0
}

// collect walks the tree, turning every node at splitLevel into memories.
// ancestors carries the heading chain above, for the kind cascade.
func collect(n *node, splitLevel int, ancestors []*node, opt Options, res *Result) {
	// Prose sitting directly under a heading ABOVE the cut belongs to nobody
	// once the cut is made. Emitting it is not tidiness: dropping it is silent
	// content loss, which is the failure this whole project is built against.
	// A test that only checked the split sections would never have seen it.
	emitLead(n, ancestors, opt, res)

	for _, kid := range n.kids {
		if kid.level == splitLevel {
			emit(kid, append(ancestors, n), opt, res)
			continue
		}
		if kid.level < splitLevel {
			collect(kid, splitLevel, append(ancestors, n), opt, res)
		}
		// kid.level > splitLevel cannot happen: a deeper heading is always a
		// descendant of one at splitLevel and is consumed by emit.
	}
}

// emitLead saves the prose attached to a heading that is not itself a cut point.
func emitLead(n *node, ancestors []*node, opt Options, res *Result) {
	lead := strings.TrimSpace(strings.Join(n.lead, "\n"))
	if lead == "" {
		return
	}
	title := n.title
	if n.level == 0 { // the preamble before the first heading has no title
		title, lead = firstLineAsTitle(lead)
		if lead == "" {
			return
		}
	}
	res.Memories = append(res.Memories, mkMemory(title, lead, resolveKind(n, ancestors, opt), false, opt))
}

// emit turns one section into one memory, or into several if it is oversized
// and has sub-headings to cut on.
func emit(n *node, ancestors []*node, opt Options, res *Result) {
	kind := resolveKind(n, ancestors, opt)
	body := strings.TrimSpace(render(n))

	if body == "" {
		res.Skipped++
		return
	}

	tokens := store.EstimateTokens(n.title) + store.EstimateTokens(body)
	if tokens <= opt.MaxTokens || len(n.kids) == 0 {
		res.Memories = append(res.Memories, mkMemory(n.title, body, kind, false, opt))
		return
	}

	// Oversized and splittable. The lead paragraph keeps the parent's title;
	// each child becomes "Parent — Child" so the context is not lost when the
	// section it belonged to is gone.
	lead := strings.TrimSpace(strings.Join(n.lead, "\n"))
	if lead != "" {
		res.Memories = append(res.Memories, mkMemory(n.title, lead, kind, true, opt))
	}
	for _, kid := range n.kids {
		sub := *kid
		sub.title = n.title + " — " + kid.title
		if kid.kind == "" {
			sub.kind = n.kind
		}
		emitSplit(&sub, kind, opt, res)
	}
}

// emitSplit is emit's recursive half: it already knows the kind and always
// marks its output as split. Recursion terminates because heading levels are
// bounded at 6, so kids run out.
func emitSplit(n *node, kind string, opt Options, res *Result) {
	body := strings.TrimSpace(render(n))
	if body == "" {
		res.Skipped++
		return
	}
	if n.kind != "" && store.Kinds[n.kind] {
		kind = n.kind
	}
	tokens := store.EstimateTokens(n.title) + store.EstimateTokens(body)
	if tokens <= opt.MaxTokens || len(n.kids) == 0 {
		res.Memories = append(res.Memories, mkMemory(n.title, body, kind, true, opt))
		return
	}
	lead := strings.TrimSpace(strings.Join(n.lead, "\n"))
	if lead != "" {
		res.Memories = append(res.Memories, mkMemory(n.title, lead, kind, true, opt))
	}
	for _, kid := range n.kids {
		sub := *kid
		sub.title = n.title + " — " + kid.title
		emitSplit(&sub, kind, opt, res)
	}
}

// render rebuilds a node's content, keeping descendant headings inline so the
// body reads as the author wrote it.
func render(n *node) string {
	var b strings.Builder
	b.WriteString(strings.Join(n.lead, "\n"))
	for _, kid := range n.kids {
		b.WriteString("\n")
		b.WriteString(strings.Repeat("#", kid.level))
		b.WriteString(" ")
		b.WriteString(kid.title)
		b.WriteString("\n")
		b.WriteString(render(kid))
	}
	return b.String()
}

// ---------------------------------------------------------------- delimited

// parseDelimited cuts plain text on `---` lines. The first non-empty line of
// each block is the title and the rest is the body; a one-line block is its own
// title, because a single sentence is a perfectly good fact.
func parseDelimited(content string, opt Options) (Result, error) {
	var res Result
	for _, raw := range splitOnRule(content) {
		block := strings.TrimSpace(raw)
		if block == "" {
			continue
		}
		title, body := firstLineAsTitle(block)
		res.Memories = append(res.Memories, mkMemory(title, body, fallbackKind("", opt), false, opt))
	}
	return res, nil
}

func splitOnRule(content string) []string {
	var (
		out   []string
		cur   []string
		fence bool
	)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if fenceRe.MatchString(line) {
			fence = !fence
		}
		if !fence && isRule(line) {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return append(out, strings.Join(cur, "\n"))
}

// isRule matches a horizontal rule: three or more dashes and nothing else.
func isRule(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return false
	}
	return strings.Trim(t, "-") == ""
}

func firstLineAsTitle(block string) (title, body string) {
	lines := strings.Split(block, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return "", ""
	}
	title = strings.TrimSpace(strings.TrimLeft(lines[i], "#- "))
	body = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
	if body == "" {
		body = title // a single sentence: the fact IS the title
	}
	return title, body
}

// ---------------------------------------------------------------- kinds

// kindAliases maps section headings onto the five kinds vestigio stores.
//
// Keys are ACCENT-FREE singulars, because lookup normalises before matching.
// Spanish plurals of -ión words drop the accent — "restricción" becomes
// "restricciones" — so a table keyed on the accented singular misses the plural
// that people actually write as a heading. That was a real miss: `# Restricciones`
// went unrecognised, level 1 stopped looking like a labelling layer, and the
// document got cut in the wrong place.
var kindAliases = map[string]string{
	"decision": "decision", "decision record": "decision", "adr": "decision",
	"architecture decision": "decision", "arquitectura": "decision",

	"bugfix": "bugfix", "bug": "bugfix", "fix": "bugfix", "error": "bugfix",
	"defect": "bugfix", "issue": "bugfix", "correccion": "bugfix",

	"pattern": "pattern", "patron": "pattern", "convention": "pattern",
	"convencion": "pattern", "practice": "pattern", "gotcha": "pattern",
	"practica": "pattern",

	"constraint": "constraint", "restriccion": "constraint", "rule": "constraint",
	"regla": "constraint", "limitation": "constraint", "requirement": "constraint",
	"requisito": "constraint", "limitacion": "constraint",

	"reference": "reference", "referencia": "reference", "link": "reference",
	"resource": "reference", "recurso": "reference", "enlace": "reference",
}

// accents folds the vowels Spanish headings actually use. Not a general
// unicode normaliser — just enough to make the alias table hold.
var accents = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
)

// kindFromTitle reads a kind out of a section heading like `# Decisions`.
//
// It only matches a heading that IS a kind name, never one that merely contains
// the word — "Decisions we regret" is a heading, not a label, and misreading it
// would silently relabel everything under it.
func kindFromTitle(title string) string {
	t := accents.Replace(strings.ToLower(strings.TrimSpace(title)))
	t = strings.Trim(t, ":.# ")

	for _, form := range []string{
		t,
		strings.TrimSuffix(t, "es"), // decisiones -> decision, errores -> error
		strings.TrimSuffix(t, "s"),  // decisions -> decision, reglas -> regla
	} {
		if form == "" {
			continue
		}
		if k, ok := kindAliases[form]; ok {
			return k
		}
	}
	return ""
}

// splitMarker pulls an explicit `[decision]` prefix off a heading. An unknown
// marker is left alone: `[WIP] Something` is a title, not a bad kind.
func splitMarker(title string) (kind, rest string) {
	m := markerRe.FindStringSubmatch(title)
	if m == nil {
		return "", title
	}
	k := strings.ToLower(m[1])
	if !store.Kinds[k] {
		return "", title
	}
	return k, strings.TrimSpace(m[2])
}

// resolveKind runs the cascade: explicit marker on the section, then the
// nearest ancestor heading that names a kind, then the command's --kind, then
// reference.
func resolveKind(n *node, ancestors []*node, opt Options) string {
	if n.kind != "" {
		return n.kind
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		if ancestors[i].kind != "" {
			return ancestors[i].kind
		}
	}
	return fallbackKind("", opt)
}

func fallbackKind(found string, opt Options) string {
	switch {
	case found != "":
		return found
	case opt.Kind != "":
		return opt.Kind
	default:
		return "reference"
	}
}

func mkMemory(title, body, kind string, split bool, opt Options) Memory {
	if title == "" {
		title = "(untitled)"
	}
	tokens := store.EstimateTokens(title) + store.EstimateTokens(body)
	return Memory{
		Title:     title,
		Body:      body,
		Kind:      kind,
		Tokens:    tokens,
		Split:     split,
		Oversized: tokens > opt.MaxTokens,
	}
}
