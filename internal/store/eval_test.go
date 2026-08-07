package store

import (
	"fmt"
	"strings"
	"testing"
)

// This file is an evaluation set, not a unit test.
//
// Every other test here asks "does search do what the code says?". This one asks
// the only question a memory system is actually judged on: when a later session
// asks in ITS OWN WORDS, does the right memory come back?
//
// That distinction is not academic. M0 found recall missing a memory for the
// query "sistema de recuerdos persistente en Golang" while every unit test was
// green and coverage said the search path was exercised. Coverage cannot see
// this class of failure, and neither can mutation testing: no mutant survives
// or dies based on whether "Golang" retrieves a memory that says "Go".
//
// The floors below are a RATCHET, not an aspiration. They are set to what the
// engine measurably does today, so a change that makes recall worse fails the
// build, and a change that makes it better is expected to raise them.

// corpus is a realistic slice of this project's own memory: short facts, mixed
// Spanish and English, the way they are actually written.
var corpus = []struct{ kind, title, body string }{
	{"decision", "Elegido Go para el binario",
		"Binario estático, sin CGO, arranque en frío rápido. Descartado Node por el peso de node_modules."},
	{"decision", "SQLite con FTS5 para el almacenamiento",
		"Base embebida en un solo archivo, texto completo sin servidor externo. modernc.org/sqlite para evitar CGO."},
	{"pattern", "Tres tools en el servidor MCP, no once",
		"El perfil de tools se paga en cada sesión se use o no. Todo lo operativo vive en el CLI, donde cuesta cero contexto."},
	{"bugfix", "Corregido el crash de FTS5 con guiones en la query",
		"MATCH es un lenguaje de consulta, no un LIKE. Un término con un operador adentro rompía la búsqueda entera. Fix: citar cada término."},
	{"constraint", "budget_tokens es un techo duro",
		"El empaquetado corta en el límite que pidió quien llama. Nunca es una sugerencia ni un objetivo."},
	{"reference", "Ledger de recibos en .rdd/ledger.md",
		"Cada unidad de trabajo deja un recibo: qué cambió, qué corrió con salida real, y qué no prueba."},
	{"decision", "Diferido el mutation testing",
		"Primero subir cobertura en mcp y retrieve. Con tests ausentes, un runner de mutantes no dice nada que go test -cover no diga en tres segundos."},
	{"bugfix", "Arreglado el binario que no arrancaba en macOS",
		"Go 1.22 generaba binarios sin LC_UUID que el dyld actual rechaza: compilaban y abortaban al arrancar. Subido a Go 1.25."},
	{"decision", "Protección de main con checks obligatorios",
		"Push directo rechazado, todo entra por pull request con siete checks verdes."},
	{"pattern", "Dedupe automático al guardar",
		"Guardar contenido idéntico actualiza la memoria existente en lugar de duplicarla y partir el recall entre casi-copias."},
	{"reference", "Importador desde engram",
		"Migración de memorias del sistema anterior, con mapeo de nombres de proyecto viejos a nuevos."},
	{"constraint", "Diagnósticos van a stderr, nunca a stdout",
		"stdout transporta JSON-RPC y nada más. Una línea de log suelta desincroniza al cliente."},
}

// evalCase is one query a real session might ask, paired with the memory it
// should surface. Queries deliberately avoid the memory's own wording — echoing
// the title back is not retrieval, it is a lookup.
type evalCase struct {
	query string
	want  string // substring of the expected memory's title
	note  string
}

var evalSet = []evalCase{
	{"sistema de recuerdos persistente en Golang", "Elegido Go", "el caso de M0: Golang vs Go, recuerdos vs memoria"},
	{"por qué descartamos node", "Elegido Go", "razón registrada en el body, no en el título"},
	{"base de datos embebida en un archivo", "SQLite", "paráfrasis directa del body"},
	{"cuántas herramientas expone el servidor", "Tres tools", "herramientas vs tools: sinónimo cruzando idioma"},
	{"la búsqueda se rompía con caracteres raros", "crash de FTS5", "síntoma, no causa"},
	{"límite de tokens en la respuesta", "budget_tokens", "concepto en español, término en inglés"},
	{"dónde quedan registradas las corridas", "Ledger de recibos", "función, no nombre"},
	{"por qué todavía no corremos mutantes", "Diferido el mutation testing", "casi literal: debería ser el caso fácil"},
	{"el binario moría en mac", "no arrancaba en macOS", "mac vs macOS, morir vs no arrancar"},
	{"no se puede pushear directo a main", "Protección de main", "efecto observable de la decisión"},
	{"guardar dos veces el mismo hecho", "Dedupe automático", "descripción del comportamiento"},
	{"por qué no imprimir logs por salida estándar", "stderr", "la restricción dicha al revés"},
	{"migrar memorias del sistema viejo", "Importador desde engram", "paráfrasis del body"},
	{"arranque en frío", "Elegido Go", "término literal del body: control positivo, debe pasar"},
	{"mutation testing", "Diferido el mutation testing", "término literal del título: control positivo, debe pasar"},
}

func seedCorpus(t *testing.T) *Store {
	t.Helper()
	s := open(t)
	for _, m := range corpus {
		if _, _, err := s.Remember("eval", m.kind, m.title, m.body); err != nil {
			t.Fatalf("seed %q: %v", m.title, err)
		}
	}
	return s
}

// rankOf reports the 0-based position of the wanted memory, or -1 if the engine
// never returned it at all.
func rankOf(hits []Memory, want string) int {
	for i, h := range hits {
		if strings.Contains(h.Title, want) {
			return i
		}
	}
	return -1
}

// TestRecallQualityBaseline measures recall@1 and recall@3 over the eval set and
// holds the line at today's numbers.
func TestRecallQualityBaseline(t *testing.T) {
	s := seedCorpus(t)

	// Measured baseline, 2026-08-07, BM25 over FTS5 with OR'd terms.
	// Counts, not percentages: a float floor invites rounding arguments, and
	// "we used to find 10 of these and now we find 9" is the actual regression.
	const (
		minAt1 = 10
		minAt3 = 12
	)

	var at1, at3, missed int
	var report strings.Builder
	report.WriteString("\nrank  query -> expected\n")

	for _, c := range evalSet {
		hits, err := s.Search("eval", c.query, 10)
		if err != nil {
			t.Fatalf("search %q: %v", c.query, err)
		}
		r := rankOf(hits, c.want)
		switch {
		case r == 0:
			at1++
			at3++
		case r > 0 && r < 3:
			at3++
		case r < 0:
			missed++
		}

		pos := "MISS"
		if r >= 0 {
			pos = fmt.Sprintf("#%d", r+1)
		}
		fmt.Fprintf(&report, "%-5s %q -> %q  (%s)\n", pos, c.query, c.want, c.note)
	}

	n := float64(len(evalSet))
	r1, r3 := float64(at1)/n, float64(at3)/n
	report.WriteString(fmt.Sprintf(
		"\nrecall@1 = %.0f%% (%d/%d)   recall@3 = %.0f%% (%d/%d)   not retrieved at all = %d\n",
		r1*100, at1, len(evalSet), r3*100, at3, len(evalSet), missed))
	t.Log(report.String())

	if at1 < minAt1 {
		t.Errorf("recall@1 fell to %d/%d, floor is %d — retrieval got WORSE; the table above says which query broke",
			at1, len(evalSet), minAt1)
	}
	if at3 < minAt3 {
		t.Errorf("recall@3 fell to %d/%d, floor is %d", at3, len(evalSet), minAt3)
	}
	if at1 > minAt1 || at3 > minAt3 {
		t.Errorf("retrieval IMPROVED to %d/%d @1 and %d/%d @3 — raise the floors to lock the gain in",
			at1, len(evalSet), at3, len(evalSet))
	}
}

// A query whose words appear verbatim in a memory must rank it first. If this
// breaks, the engine is not doing lexical matching at all and nothing above is
// meaningful.
func TestRecallExactTermsRankFirst(t *testing.T) {
	s := seedCorpus(t)
	cases := map[string]string{
		"LC_UUID":      "macOS",
		"node_modules": "Elegido Go",
		"JSON-RPC":     "stderr",
		"pull request": "Protección de main",
		"casi-copias":  "Dedupe",
	}
	for query, want := range cases {
		t.Run(query, func(t *testing.T) {
			hits, err := s.Search("eval", query, 10)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if r := rankOf(hits, want); r != 0 {
				t.Errorf("query %q put %q at rank %d, want rank 0 (hits: %s)", query, want, r+1, titles(hits))
			}
		})
	}
}

// A term nobody wrote must return nothing. An engine that always returns
// something is worse than one that admits it found nothing: the agent cannot
// tell a real memory from noise.
func TestRecallReturnsNothingForAbsentTerms(t *testing.T) {
	s := seedCorpus(t)
	for _, q := range []string{"kubernetes", "reciprocal", "facturación"} {
		hits, err := s.Search("eval", q, 10)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		if len(hits) != 0 {
			t.Errorf("query %q returned %d hits, want none: %s", q, len(hits), titles(hits))
		}
	}
}

// Search ORs its terms, so one shared word is enough to be returned. That is a
// deliberate recall-over-precision trade — this test pins the consequence so the
// trade stays visible instead of being rediscovered as a bug.
func TestRecallIsGenerousBecauseTermsAreORed(t *testing.T) {
	s := seedCorpus(t)
	hits, err := s.Search("eval", "el binario de kubernetes", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected the shared term 'binario' to still match; OR semantics changed")
	}
	// The destructive path must NOT be this generous.
	all, err := s.SearchAll("eval", "el binario de kubernetes", 10)
	if err != nil {
		t.Fatalf("searchAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("SearchAll matched %d memories for a query with an absent term: %s", len(all), titles(all))
	}
}

func titles(hits []Memory) string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Title)
	}
	if len(out) == 0 {
		return "(none)"
	}
	return strings.Join(out, " | ")
}
