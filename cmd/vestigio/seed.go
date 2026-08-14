package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/valzkat1/vestigio/internal/seed"
	"github.com/valzkat1/vestigio/internal/store"
)

// seed bootstraps a store from documents the user already wrote.
//
// It is a CLI command and not an MCP tool for the same reason import is: an
// agent never needs to call it, so it costs zero context. An agent COULD read a
// document and emit remember() calls, and that is strictly worse — it pays
// thousands of tokens to read the file, it is not idempotent, and it happens
// after a session has already started. Seeding is a day-zero operation.
//
// What belongs here is knowledge that was ACQUIRED and happens to live in a
// file: decision records, gotcha lists, post-mortems, onboarding notes. What
// does NOT belong here is rules — "use strict mode", "no any". Those are
// AGENTS.md, which is already in the agent's context for free; copying them
// into memory buys a recall call to learn something the agent was told anyway.
var seedSpec = argSpec{
	usage: "usage: vestigio seed <file.md|file.txt> [--project=NAME] [--kind=KIND] " +
		"[--split=N] [--max-tokens=N] [--dry-run]",
	value:   []string{"project", "kind", "split", "max-tokens"},
	boolean: []string{"dry-run"},
	maxPos:  1,
}

func runSeed(args []string) int {
	a, err := parseArgs(args, seedSpec)
	if err != nil {
		return usageErr(err)
	}
	if len(a.pos) == 0 {
		return usageErr(fmt.Errorf("no file given\n%s", seedSpec.usage))
	}
	path := a.pos[0]

	opt, code := seedOptions(a)
	if code != 0 {
		return code
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fail(fmt.Errorf("read %s: %w", path, err))
	}

	res, err := seed.Parse(filepath.Base(path), string(raw), opt)
	if err != nil {
		return usageErr(err)
	}
	if len(res.Memories) == 0 {
		fmt.Printf("%s produced no memories — nothing written\n", path)
		if res.Skipped > 0 {
			fmt.Printf("%d heading(s) had no content under them\n", res.Skipped)
		}
		return 0
	}

	project := ""
	if p, ok := a.value("project"); ok {
		project = p
	}
	if project == "" {
		project = detectProject()
	}

	fmt.Printf("%s → project %q%s\n\n", path, project, cutDescription(res))
	if a.has("dry-run") {
		return seedDryRun(res)
	}
	return seedWrite(res, project)
}

// seedOptions validates the flags before anything is opened or read. A bad
// --kind must not cost the user a half-written store.
func seedOptions(a *cmdArgs) (seed.Options, int) {
	var opt seed.Options

	if k, ok := a.value("kind"); ok {
		if !store.Kinds[k] {
			return opt, usageErr(fmt.Errorf("unknown kind %q — valid kinds are %s", k, validKinds()))
		}
		opt.Kind = k
	}
	if s, ok := a.value("split"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 6 {
			return opt, usageErr(fmt.Errorf("--split needs a heading level between 1 and 6, got %q", s))
		}
		opt.SplitLevel = n
	}
	if s, ok := a.value("max-tokens"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return opt, usageErr(fmt.Errorf("--max-tokens needs a positive whole number, got %q", s))
		}
		opt.MaxTokens = n
	}
	return opt, 0
}

func cutDescription(res seed.Result) string {
	if res.SplitLevel == 0 {
		return ", cut on `---` rules"
	}
	return fmt.Sprintf(", cut at H%d", res.SplitLevel)
}

// seedDryRun prints the plan. Nothing is opened, so a dry run cannot touch the
// store even by accident.
func seedDryRun(res seed.Result) int {
	fmt.Println("DRY RUN — nothing is written")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tKIND\tTOKENS\tTITLE")
	total := 0
	for i, m := range res.Memories {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", seedMark(m), m.Kind, m.Tokens, truncate(m.Title, 58))
		total += m.Tokens
		_ = i
	}
	w.Flush()

	fmt.Printf("\n%d memories · %d tokens\n", len(res.Memories), total)
	seedNotes(res)
	return 0
}

func seedWrite(res seed.Result, project string) int {
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	var created, merged, total int
	for _, m := range res.Memories {
		id, isNew, err := st.Remember(project, m.Kind, m.Title, m.Body)
		if err != nil {
			w.Flush()
			return fail(fmt.Errorf("store %q: %w", m.Title, err))
		}
		action := "merged"
		if isNew {
			created++
			action = "created"
		} else {
			merged++
		}
		fmt.Fprintf(w, "%s\t%s\t#%d\t[%s]\t%s\n", seedMark(m), action, id, m.Kind, truncate(m.Title, 52))
		total += m.Tokens
	}
	w.Flush()

	fmt.Printf("\n%d memories: %d created, %d merged · %d tokens\n",
		len(res.Memories), created, merged, total)
	seedNotes(res)
	return 0
}

// seedMark flags rows the operator should look at twice.
func seedMark(m seed.Memory) string {
	switch {
	case m.Oversized:
		return "!"
	case m.Split:
		return "*"
	default:
		return " "
	}
}

// seedNotes explains the marks and the skips.
//
// Auto-splitting invents titles the author never wrote, and an oversized memory
// cannot be returned within a normal budget. Both are defensible outcomes and
// neither should be silent.
func seedNotes(res seed.Result) {
	var split, oversized int
	for _, m := range res.Memories {
		if m.Oversized {
			oversized++
		} else if m.Split {
			split++
		}
	}
	if split > 0 {
		fmt.Printf("* %d memory(ies) came from splitting an oversized section — those titles were generated, not written\n", split)
	}
	if oversized > 0 {
		fmt.Printf("! %d memory(ies) exceed the size limit and had no sub-headings to split on.\n"+
			"  They are stored, but a single memory that large will crowd out everything else in a recall.\n", oversized)
	}
	if res.Skipped > 0 {
		fmt.Printf("%d heading(s) skipped for having no content — a title alone is not a fact\n", res.Skipped)
	}
}
