package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/valzkat1/vestigio/internal/store"
)

// The admin commands are CLI-only, for the same reason import is: an agent never
// needs to browse or hand-edit the store, and every tool exposed over MCP is
// schema text paid for in every session of every agent.
//
// They operate on ids globally rather than within the detected project. An id is
// already unique database-wide, and scoping it would mean that running the CLI
// from the wrong directory reports "not found" for a row that plainly exists —
// the same silent-wrong-project failure detectProject warns about. The project
// is printed on every result so the operator sees what they are touching.

func openStore() (*store.Store, error) { return store.Open(store.DefaultPath()) }

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "vestigio:", err)
	return 1
}

func fmtTime(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

// truncate cuts on runes, not bytes — titles are routinely non-ASCII here.
func truncate(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// argSpec declares the entire surface a command accepts.
//
// The helpers this replaced scanned the arguments for what they expected and
// ignored everything else, which meant `list --porject=other` listed the
// detected project and read like an answer, and `rm 1 2 --yes` deleted one
// memory while reporting success. Both are the failure this project is built
// against: a wrong answer delivered quietly.
//
// parseImportArgs already worked this way for `import`. This is the same rule,
// applied to the commands that browse and destroy.
type argSpec struct {
	usage   string
	value   []string // flags carrying a value inline, after "="
	boolean []string // flags that are simply present or absent
	maxPos  int      // positional arguments the command can actually use
}

type cmdArgs struct {
	values map[string]string
	flags  map[string]bool
	pos    []string
}

func (a *cmdArgs) value(name string) (string, bool) { v, ok := a.values[name]; return v, ok }
func (a *cmdArgs) has(name string) bool             { return a.flags[name] }

// id reads the single positional every id-taking command needs.
func (a *cmdArgs) id() (int64, error) {
	if len(a.pos) == 0 {
		return 0, fmt.Errorf("no id given")
	}
	id, err := strconv.ParseInt(a.pos[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", a.pos[0])
	}
	return id, nil
}

func parseArgs(args []string, spec argSpec) (*cmdArgs, error) {
	takesValue, isBool := map[string]bool{}, map[string]bool{}
	for _, f := range spec.value {
		takesValue[f] = true
	}
	for _, f := range spec.boolean {
		isBool[f] = true
	}

	out := &cmdArgs{values: map[string]string{}, flags: map[string]bool{}}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out.pos = append(out.pos, a)
			continue
		}
		name, val, inline := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		switch {
		case !strings.HasPrefix(a, "--"):
			return nil, fmt.Errorf("unknown flag %q — flags are long form\n%s", a, spec.usage)
		case inline && takesValue[name]:
			out.values[name] = val
		case !inline && isBool[name]:
			out.flags[name] = true
		case !inline && takesValue[name]:
			return nil, fmt.Errorf("--%s takes its value inline: --%s=VALUE\n%s", name, name, spec.usage)
		case inline && isBool[name]:
			return nil, fmt.Errorf("--%s takes no value\n%s", name, spec.usage)
		default:
			return nil, fmt.Errorf("unknown flag %q\n%s", "--"+name, spec.usage)
		}
	}

	if len(out.pos) > spec.maxPos {
		if spec.maxPos == 0 {
			return nil, fmt.Errorf("unexpected argument %q — this command takes none\n%s", out.pos[0], spec.usage)
		}
		// Silently keeping one of two is how the import bug did its damage, and
		// here the discarded one would have been a memory the operator asked to
		// delete.
		return nil, fmt.Errorf("unexpected argument %q — this command takes one at a time, and %d were given\n%s",
			out.pos[spec.maxPos], len(out.pos), spec.usage)
	}
	return out, nil
}

// usageErr reports a malformed command line. Exit 2 is the usage code the rest
// of the CLI already uses, and it is distinct from 1 so a script can tell "you
// typed it wrong" from "it ran and failed".
func usageErr(err error) int {
	fmt.Fprintln(os.Stderr, "vestigio:", err)
	return 2
}

var (
	projectsSpec = argSpec{usage: "usage: vestigio projects"}
	listSpec     = argSpec{
		usage:   "usage: vestigio list [--project=NAME] [--all] [--kind=KIND] [--limit=N]",
		value:   []string{"project", "kind", "limit"},
		boolean: []string{"all"},
	}
	showSpec = argSpec{usage: "usage: vestigio show <id>", maxPos: 1}
	editSpec = argSpec{
		usage:   "usage: vestigio edit <id> [--title=T] [--kind=K] [--body-file=F] [--fix]",
		value:   []string{"title", "kind", "body-file"},
		boolean: []string{"fix"},
		maxPos:  1,
	}
	rmSpec     = argSpec{usage: "usage: vestigio rm <id> [--yes]", boolean: []string{"yes"}, maxPos: 1}
	verifySpec = argSpec{usage: "usage: vestigio verify"}
)

// validKinds lists the closed set for error messages, sorted so the text does
// not change between runs over a map.
func validKinds() string {
	ks := make([]string, 0, len(store.Kinds))
	for k := range store.Kinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, "|")
}

// shortHash trims a hash for display without assuming it is well formed.
//
// A drifted row carries whatever a SQL prompt or a GUI browser left behind, and
// slicing it blind panicked on anything shorter than the cut — on `edit --fix`,
// the one command `verify` tells the operator to run to repair exactly those
// rows. The repair path must survive the worst row it will ever meet.
func shortHash(h string) string {
	if h == "" {
		return "(empty)"
	}
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func runProjects(args []string) int {
	if _, err := parseArgs(args, projectsSpec); err != nil {
		return usageErr(err)
	}
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	rows, err := st.Projects()
	if err != nil {
		return fail(err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tMEMORIES\tTOKENS")
	total, tokens := 0, 0
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%d\t%d\n", r.Project, r.Count, r.Tokens)
		total, tokens = total+r.Count, tokens+r.Tokens
	}
	fmt.Fprintf(w, "\t%d\t%d\n", total, tokens)
	w.Flush()
	fmt.Printf("\ndatabase: %s\n", store.DefaultPath())
	return 0
}

func runList(args []string) int {
	// Everything is validated before the store is opened: a value the command
	// cannot honour must stop it, never be quietly swapped for a default.
	a, err := parseArgs(args, listSpec)
	if err != nil {
		return usageErr(err)
	}
	kind, _ := a.value("kind")
	if kind != "" && !store.Kinds[kind] {
		return usageErr(fmt.Errorf("invalid kind %q — valid kinds are %s", kind, validKinds()))
	}
	limit := 30
	if v, ok := a.value("limit"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			return usageErr(fmt.Errorf("--limit needs a positive whole number, got %q", v))
		}
		limit = n
	}

	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	project := detectProject()
	if v, ok := a.value("project"); ok {
		project = v
	}
	if a.has("all") {
		project = ""
	}

	rows, err := st.List(project, kind, limit)
	if err != nil {
		return fail(err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tTOKENS\tUPDATED\tTITLE")
	for _, r := range rows {
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\n",
			r.ID, r.Kind, r.Tokens, fmtTime(r.UpdatedAt), truncate(r.Title, 60))
	}
	w.Flush()

	scope := project
	if scope == "" {
		scope = "all projects"
	}
	fmt.Printf("\n%d memories — %s\n", len(rows), scope)
	if len(rows) == 0 && project != "" {
		fmt.Printf("nothing filed under %q. `vestigio projects` lists every key.\n", project)
	}
	return 0
}

func runShow(args []string) int {
	a, err := parseArgs(args, showSpec)
	if err != nil {
		return usageErr(err)
	}
	id, err := a.id()
	if err != nil {
		return usageErr(err)
	}
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	d, err := st.Get(id)
	if err != nil {
		return fail(err)
	}
	if d == nil {
		return fail(fmt.Errorf("no memory with id %d", id))
	}

	fmt.Printf("#%d  [%s]  project %s\n", d.ID, d.Kind, d.Project)
	fmt.Printf("title    %s\n", d.Title)
	fmt.Printf("tokens   %d\nhash     %s\n", d.Tokens, d.Hash)
	fmt.Printf("created  %s\nupdated  %s\n", fmtTime(d.CreatedAt), fmtTime(d.UpdatedAt))
	fmt.Printf("recalled %d time(s), last %s\n", d.RecallCount, fmtTime(d.RecalledAt))
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println(d.Body)
	return 0
}

// runEdit rewrites a memory through the store so hash and tokens are recomputed.
//
// With no content flags it opens $VISUAL/$EDITOR on the current body, which is
// what "edit manually" usually means. --fix skips the editor and just rewrites
// the row, repairing a drifted hash without changing a character of content.
func runEdit(args []string) int {
	a, err := parseArgs(args, editSpec)
	if err != nil {
		return usageErr(err)
	}
	id, err := a.id()
	if err != nil {
		return usageErr(err)
	}
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	cur, err := st.Get(id)
	if err != nil {
		return fail(err)
	}
	if cur == nil {
		return fail(fmt.Errorf("no memory with id %d", id))
	}

	title, _ := a.value("title")
	kind, _ := a.value("kind")
	body := ""

	if f, ok := a.value("body-file"); ok {
		b, err := os.ReadFile(f)
		if err != nil {
			return fail(err)
		}
		body = string(b)
	} else if title == "" && kind == "" && !a.has("fix") {
		body, err = editInEditor(cur.Body)
		if err != nil {
			return fail(err)
		}
		if strings.TrimSpace(body) == strings.TrimSpace(cur.Body) {
			fmt.Println("no changes")
			return 0
		}
	}

	updated, err := st.Update(id, title, kind, body)
	if err != nil {
		return fail(err)
	}

	fmt.Printf("#%d updated\n", updated.ID)
	if updated.Tokens != cur.Tokens {
		fmt.Printf("  tokens %d -> %d\n", cur.Tokens, updated.Tokens)
	}
	if updated.Hash != cur.Hash {
		fmt.Printf("  hash   %s… -> %s…\n", shortHash(cur.Hash), shortHash(updated.Hash))
	} else {
		fmt.Println("  hash unchanged — content is identical")
	}
	fmt.Println("  FTS index resynced by the memories_au trigger")
	return 0
}

func editInEditor(current string) (string, error) {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		return "", fmt.Errorf("no $VISUAL or $EDITOR set — use --body-file=FILE instead")
	}

	f, err := os.CreateTemp("", "vestigio-*.md")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(current); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	parts := strings.Fields(ed)
	cmd := exec.Command(parts[0], append(parts[1:], filepath.Clean(path))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %q: %w", ed, err)
	}

	out, err := os.ReadFile(path)
	return string(out), err
}

func runRm(args []string) int {
	a, err := parseArgs(args, rmSpec)
	if err != nil {
		return usageErr(err)
	}
	id, err := a.id()
	if err != nil {
		return usageErr(err)
	}
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	d, err := st.Get(id)
	if err != nil {
		return fail(err)
	}
	if d == nil {
		return fail(fmt.Errorf("no memory with id %d", id))
	}

	if !a.has("yes") {
		fmt.Printf("#%d [%s] %s\n  project %s, %d tokens\n",
			d.ID, d.Kind, d.Title, d.Project, d.Tokens)
		fmt.Print("delete? [y/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Println("cancelled")
			return 0
		}
	}

	n, err := st.ForgetID(d.Project, id)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("%d memory deleted\n", n)
	return 0
}

// runVerify reports rows whose hash or token count no longer matches the content.
func runVerify(args []string) int {
	if _, err := parseArgs(args, verifySpec); err != nil {
		return usageErr(err)
	}
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	drift, err := st.Verify()
	if err != nil {
		return fail(err)
	}
	if len(drift) == 0 {
		fmt.Println("ok — every hash and token count matches its content")
		return 0
	}
	fmt.Printf("%d row(s) drifted:\n", len(drift))
	for _, d := range drift {
		what := []string{}
		if d.HashStale {
			what = append(what, "hash")
		}
		if d.TokensHave != d.TokensWant {
			what = append(what, fmt.Sprintf("tokens %d->%d", d.TokensHave, d.TokensWant))
		}
		fmt.Printf("  #%d: %s\n", d.ID, strings.Join(what, ", "))
	}
	fmt.Println("\nrepair with: vestigio edit <id> --fix")
	return 1
}
