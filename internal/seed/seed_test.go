package seed

import (
	"strings"
	"testing"
)

// What these tests are for: the cut is the product. Reading a file is trivial
// and untestable-in-anger; deciding that THIS heading is a memory and THAT one
// is a detail inside it is the whole design, and it is where a wrong answer
// gets delivered quietly — a seeded store full of half-facts looks fine until
// recall returns one.

func titles(ms []Memory) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Title)
	}
	return out
}

func mustParse(t *testing.T, name, content string, opt Options) Result {
	t.Helper()
	res, err := Parse(name, content, opt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return res
}

func TestCutsMarkdownOnRepeatedHeadingLevel(t *testing.T) {
	doc := `# Decisions

## Elegimos Go para el binario

Binario estático, sin CGO. Descartamos Node por el peso de node_modules.

## SQLite con FTS5 para el almacenamiento

Base embebida en un solo archivo, texto completo sin servidor externo.
`
	res := mustParse(t, "decisions.md", doc, Options{})

	if res.SplitLevel != 2 {
		t.Errorf("split level = %d, want 2 (H1 appears once, H2 twice)", res.SplitLevel)
	}
	if len(res.Memories) != 2 {
		t.Fatalf("got %d memories, want 2: %v", len(res.Memories), titles(res.Memories))
	}
	if got := res.Memories[0].Title; got != "Elegimos Go para el binario" {
		t.Errorf("title = %q", got)
	}
	if !strings.Contains(res.Memories[0].Body, "node_modules") {
		t.Errorf("body lost content: %q", res.Memories[0].Body)
	}
	// The H1 "Decisions" names a kind, so both children inherit it.
	for _, m := range res.Memories {
		if m.Kind != "decision" {
			t.Errorf("memory %q got kind %q, want decision inherited from the H1", m.Title, m.Kind)
		}
	}
}

// A file holding several whole records repeats H1. Cutting at the shallowest
// REPEATED level has to pick 1 here and 2 in the test above, from the same rule.
func TestCutsAtH1WhenH1Repeats(t *testing.T) {
	doc := `# ADR-001: Use Go

## Context
Node was slow to start.

## Decision
Go, static binary.

# ADR-002: Use SQLite

## Context
No server wanted.

## Decision
SQLite with FTS5.
`
	res := mustParse(t, "adr.md", doc, Options{})

	if res.SplitLevel != 1 {
		t.Fatalf("split level = %d, want 1 (H1 repeats)", res.SplitLevel)
	}
	if len(res.Memories) != 2 {
		t.Fatalf("got %d memories, want 2: %v", len(res.Memories), titles(res.Memories))
	}
	// Each ADR keeps its own sub-structure in the body rather than exploding
	// into "Context" and "Decision" as separate facts.
	if !strings.Contains(res.Memories[0].Body, "## Context") {
		t.Errorf("sub-headings were lost from the body: %q", res.Memories[0].Body)
	}
	if !strings.Contains(res.Memories[0].Body, "static binary") {
		t.Errorf("body lost content: %q", res.Memories[0].Body)
	}
}

// A level whose headings all name kinds is a labelling layer, not a layer of
// facts. Cutting there wrecks both the cut and the kind cascade.
//
// Regression test, found by running the binary on a real document: `# Decisiones`
// and `# Restricciones` repeat at level 1, so the naive "shallowest repeated
// level" rule cut there and produced two enormous memories named after the
// categories instead of five facts.
func TestCategoryHeadingsAreNotACutLevel(t *testing.T) {
	doc := `# Decisiones

## Elegimos Go
Sin CGO, arranque rápido.

## SQLite con FTS5
Base embebida.

# Restricciones

## No editar la base con SQL crudo
Los triggers no tocan hash ni tokens.

## Diagnósticos van a stderr
stdout transporta JSON-RPC y nada más.
`
	res := mustParse(t, "notas.md", doc, Options{})

	if res.SplitLevel != 2 {
		t.Fatalf("split level = %d, want 2 — level 1 is all category labels", res.SplitLevel)
	}
	if len(res.Memories) != 4 {
		t.Fatalf("got %d memories, want 4: %v", len(res.Memories), titles(res.Memories))
	}
	// And the categories still do their real job: classifying their children.
	kinds := map[string]string{}
	for _, m := range res.Memories {
		kinds[m.Title] = m.Kind
	}
	if kinds["Elegimos Go"] != "decision" {
		t.Errorf("Elegimos Go got kind %q, want decision", kinds["Elegimos Go"])
	}
	if kinds["Diagnósticos van a stderr"] != "constraint" {
		t.Errorf("stderr memory got kind %q, want constraint", kinds["Diagnósticos van a stderr"])
	}
}

// An explicit [decision] marker labels a REAL fact, so it must not make its
// level look like a labelling layer.
func TestInlineMarkersDoNotMakeALabelLayer(t *testing.T) {
	doc := "# [decision] Elegimos Go\nSin CGO.\n\n# [decision] SQLite con FTS5\nBase embebida.\n"
	res := mustParse(t, "n.md", doc, Options{})

	if res.SplitLevel != 1 {
		t.Fatalf("split level = %d, want 1 — these are facts, not categories", res.SplitLevel)
	}
	if len(res.Memories) != 2 {
		t.Fatalf("got %d memories, want 2: %v", len(res.Memories), titles(res.Memories))
	}
}

// The single-ADR file is the case detectLevel gets wrong on purpose, because
// nothing in the text distinguishes it. --split=1 is the answer, and this test
// pins that the escape hatch works.
func TestSplitLevelOverrideKeepsASingleRecordWhole(t *testing.T) {
	doc := `# ADR-001: Use Go

## Context
Node was slow.

## Decision
Go it is.
`
	auto := mustParse(t, "adr.md", doc, Options{})
	if len(auto.Memories) != 2 {
		t.Fatalf("auto-detect got %d memories, expected it to cut at H2 here", len(auto.Memories))
	}

	forced := mustParse(t, "adr.md", doc, Options{SplitLevel: 1})
	if len(forced.Memories) != 1 {
		t.Fatalf("--split=1 got %d memories, want 1: %v", len(forced.Memories), titles(forced.Memories))
	}
	if !strings.Contains(forced.Memories[0].Body, "Node was slow") {
		t.Errorf("body lost the Context section: %q", forced.Memories[0].Body)
	}
}

func TestInlineMarkerBeatsSectionKind(t *testing.T) {
	doc := `# Decisions

## Elegimos Go
Sin CGO.

## [bugfix] El binario moría en macOS
Go 1.22 generaba binarios sin LC_UUID.
`
	res := mustParse(t, "mixed.md", doc, Options{})
	if len(res.Memories) != 2 {
		t.Fatalf("got %d memories", len(res.Memories))
	}
	if res.Memories[0].Kind != "decision" {
		t.Errorf("first memory kind = %q, want decision from the section", res.Memories[0].Kind)
	}
	if res.Memories[1].Kind != "bugfix" {
		t.Errorf("second memory kind = %q, want bugfix from the inline marker", res.Memories[1].Kind)
	}
	if got := res.Memories[1].Title; got != "El binario moría en macOS" {
		t.Errorf("marker was not stripped from the title: %q", got)
	}
}

func TestKindCascadeFallsBackToFlagThenReference(t *testing.T) {
	doc := "## Algo que aprendimos\nUn detalle.\n\n## Otra cosa\nOtro detalle.\n"

	withFlag := mustParse(t, "notes.md", doc, Options{Kind: "pattern"})
	for _, m := range withFlag.Memories {
		if m.Kind != "pattern" {
			t.Errorf("memory %q got %q, want the --kind fallback", m.Title, m.Kind)
		}
	}

	bare := mustParse(t, "notes.md", doc, Options{})
	for _, m := range bare.Memories {
		if m.Kind != "reference" {
			t.Errorf("memory %q got %q, want reference as the last resort", m.Title, m.Kind)
		}
	}
}

// An unknown marker is a title, not a bad kind. `[WIP] Thing` must survive.
func TestUnknownMarkerIsLeftInTheTitle(t *testing.T) {
	res := mustParse(t, "n.md", "## [WIP] Algo a medias\nCuerpo.\n", Options{})
	m := res.Memories[0]
	if m.Title != "[WIP] Algo a medias" {
		t.Errorf("title = %q, want the marker preserved", m.Title)
	}
	if m.Kind != "reference" {
		t.Errorf("kind = %q, want reference", m.Kind)
	}
}

// A `#` inside a fenced block is a shell comment. Treating it as a heading is
// how one code sample becomes three memories.
func TestHashInsideFenceIsNotAHeading(t *testing.T) {
	doc := "## Instalación\n\n```bash\n# instalar la herramienta\ngo install ./...\n# listo\n```\n\n## Uso\nCorrer el binario.\n"
	res := mustParse(t, "guide.md", doc, Options{})

	if len(res.Memories) != 2 {
		t.Fatalf("got %d memories, want 2 — a fenced # was read as a heading: %v",
			len(res.Memories), titles(res.Memories))
	}
	if !strings.Contains(res.Memories[0].Body, "# instalar la herramienta") {
		t.Errorf("fenced comment was lost from the body: %q", res.Memories[0].Body)
	}
}

// Content under a heading ABOVE the cut has an owner in the document and none
// after the cut. It must still be stored.
//
// This is a regression test for a real bug: `collect` recursed past such nodes
// and never looked at their prose, so an intro paragraph under `# Decisions`
// vanished without a word. It surfaced by accident — the oversized-split test
// below had its level auto-detected deeper than intended, and the missing lead
// showed up in the diff of expected titles.
func TestProseAboveTheCutIsNotDropped(t *testing.T) {
	doc := `# Decisions

Este archivo lista las decisiones del proyecto.

## Elegimos Go
Sin CGO.

## SQLite con FTS5
Base embebida.
`
	res := mustParse(t, "decisions.md", doc, Options{})

	var found bool
	for _, m := range res.Memories {
		if strings.Contains(m.Body, "lista las decisiones del proyecto") {
			found = true
			if m.Title != "Decisions" {
				t.Errorf("lead memory title = %q, want the heading it sat under", m.Title)
			}
		}
	}
	if !found {
		t.Fatalf("the paragraph under the H1 was silently dropped; got: %v", titles(res.Memories))
	}
}

// Text before the very first heading has no heading at all, and is the easiest
// thing of all to lose.
func TestPreambleBeforeAnyHeadingIsKept(t *testing.T) {
	doc := "Notas sueltas del proyecto\nescritas antes de cualquier sección.\n\n## Una decisión\nAlgo.\n"
	res := mustParse(t, "n.md", doc, Options{})

	var found bool
	for _, m := range res.Memories {
		if strings.Contains(m.Body, "antes de cualquier sección") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the preamble was dropped; got: %v", titles(res.Memories))
	}
}

func TestOversizedSectionSplitsOnSubHeadings(t *testing.T) {
	big := strings.Repeat("contenido largo de relleno para pasar el umbral. ", 40)
	doc := "## Guía enorme\n\nIntroducción corta.\n\n### Primera parte\n" + big +
		"\n\n### Segunda parte\n" + big + "\n"

	// SplitLevel is forced: auto-detect would pick H3 here (H3 repeats, H2 does
	// not), which cuts the sections apart directly and never exercises the
	// oversized path this test exists for.
	res := mustParse(t, "guia.md", doc, Options{MaxTokens: 100, SplitLevel: 2})

	if len(res.Memories) < 3 {
		t.Fatalf("expected the lead plus two split children, got %d: %v",
			len(res.Memories), titles(res.Memories))
	}
	if res.Memories[0].Title != "Guía enorme" {
		t.Errorf("lead memory title = %q", res.Memories[0].Title)
	}
	if got := res.Memories[1].Title; got != "Guía enorme — Primera parte" {
		t.Errorf("split child title = %q, want the parent prefixed so context survives", got)
	}
	for _, m := range res.Memories[1:] {
		if !m.Split {
			t.Errorf("memory %q should be marked Split so the dry-run can show it", m.Title)
		}
	}
}

// The honest fallback: a big section with nothing to cut on is stored whole and
// reported. Dropping the user's content would be worse than storing it awkward.
func TestOversizedSectionWithoutSubHeadingsIsKeptAndFlagged(t *testing.T) {
	big := strings.Repeat("una sola parrafada sin subtítulos que no se puede partir. ", 40)
	res := mustParse(t, "muro.md", "## Muro de texto\n\n"+big+"\n", Options{MaxTokens: 100})

	if len(res.Memories) != 1 {
		t.Fatalf("got %d memories, want 1 — there was nothing to split on: %v",
			len(res.Memories), titles(res.Memories))
	}
	m := res.Memories[0]
	if !m.Oversized {
		t.Error("memory should be flagged Oversized so the operator is told")
	}
	if m.Split {
		t.Error("nothing was split, so Split must be false")
	}
	if !strings.Contains(m.Body, "no se puede partir") {
		t.Error("content was dropped")
	}
}

func TestHeadingWithNoContentIsSkipped(t *testing.T) {
	doc := "## Vacía\n\n## Con cuerpo\nAlgo real.\n"
	res := mustParse(t, "n.md", doc, Options{})

	if len(res.Memories) != 1 {
		t.Fatalf("got %d memories, want 1: %v", len(res.Memories), titles(res.Memories))
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 — a title alone is not a fact", res.Skipped)
	}
}

func TestPlainTextCutsOnRules(t *testing.T) {
	doc := `Elegimos Go para el binario
Binario estático, sin CGO.

---

SQLite con FTS5
Base embebida en un solo archivo.
`
	res := mustParse(t, "notas.txt", doc, Options{Kind: "decision"})

	if len(res.Memories) != 2 {
		t.Fatalf("got %d memories, want 2: %v", len(res.Memories), titles(res.Memories))
	}
	if res.Memories[0].Title != "Elegimos Go para el binario" {
		t.Errorf("title = %q, want the first line", res.Memories[0].Title)
	}
	if res.Memories[0].Body != "Binario estático, sin CGO." {
		t.Errorf("body = %q, want the rest of the block", res.Memories[0].Body)
	}
	for _, m := range res.Memories {
		if m.Kind != "decision" {
			t.Errorf("memory %q kind = %q, want the --kind flag", m.Title, m.Kind)
		}
	}
}

// A one-line block is a perfectly good fact. It must not be dropped for having
// no body, and title and body should both carry the sentence.
func TestSingleLineTextBlockKeepsItsSentence(t *testing.T) {
	res := mustParse(t, "n.txt", "No usar jest-mock-extended.\n", Options{Kind: "constraint"})
	if len(res.Memories) != 1 {
		t.Fatalf("got %d memories, want 1", len(res.Memories))
	}
	m := res.Memories[0]
	if m.Title != "No usar jest-mock-extended." || m.Body != "No usar jest-mock-extended." {
		t.Errorf("title=%q body=%q, want the sentence in both", m.Title, m.Body)
	}
}

func TestMarkdownWithoutHeadingsBecomesOneMemory(t *testing.T) {
	res := mustParse(t, "flat.md", "Primera línea como título\ny el resto es el cuerpo.\n", Options{})
	if len(res.Memories) != 1 {
		t.Fatalf("got %d memories, want 1: %v", len(res.Memories), titles(res.Memories))
	}
	if res.Memories[0].Title != "Primera línea como título" {
		t.Errorf("title = %q", res.Memories[0].Title)
	}
}

func TestEmptyInputProducesNothing(t *testing.T) {
	for _, name := range []string{"e.md", "e.txt"} {
		res := mustParse(t, name, "\n   \n\n", Options{})
		if len(res.Memories) != 0 {
			t.Errorf("%s: got %d memories from blank input", name, len(res.Memories))
		}
	}
}

func TestRejectsBadOptions(t *testing.T) {
	if _, err := Parse("n.md", "## A\nb\n", Options{Kind: "decisionn"}); err == nil {
		t.Error("an unknown kind must be rejected, not silently replaced")
	}
	if _, err := Parse("n.md", "## A\nb\n", Options{SplitLevel: 9}); err == nil {
		t.Error("a split level outside 1..6 must be rejected")
	}
}

// Tokens are what the budget packer will see. If this drifts from the store's
// own estimate, the dry-run reports a size the packer does not agree with.
func TestTokensMatchTheStoreEstimate(t *testing.T) {
	res := mustParse(t, "n.md", "## Título\nUn cuerpo de prueba con varias palabras.\n", Options{})
	m := res.Memories[0]
	if m.Tokens <= 0 {
		t.Fatalf("tokens = %d", m.Tokens)
	}
	// Same formula the store uses on write, so what the dry-run prints is what
	// recall will later count against budget_tokens.
	want := (len([]rune(m.Title))+3)/4 + (len([]rune(m.Body))+3)/4
	if m.Tokens != want {
		t.Errorf("tokens = %d, want %d (store.EstimateTokens)", m.Tokens, want)
	}
}
