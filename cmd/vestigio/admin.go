package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func flagValue(args []string, name string) (string, bool) {
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "--"+name+"="); ok {
			return v, true
		}
	}
	return "", false
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name {
			return true
		}
	}
	return false
}

// firstPositional returns the first argument that is not a flag.
func firstPositional(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

func parseID(args []string) (int64, error) {
	raw := firstPositional(args)
	if raw == "" {
		return 0, fmt.Errorf("no id given")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", raw)
	}
	return id, nil
}

func runProjects(args []string) int {
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
	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	defer st.Close()

	project := detectProject()
	if v, ok := flagValue(args, "project"); ok {
		project = v
	}
	if hasFlag(args, "all") {
		project = ""
	}
	kind, _ := flagValue(args, "kind")
	limit := 30
	if v, ok := flagValue(args, "limit"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
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
	id, err := parseID(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: vestigio show <id>")
		return 2
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
	id, err := parseID(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: vestigio edit <id> [--title=T] [--kind=K] [--body-file=F] [--fix]")
		return 2
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

	title, _ := flagValue(args, "title")
	kind, _ := flagValue(args, "kind")
	body := ""

	if f, ok := flagValue(args, "body-file"); ok {
		b, err := os.ReadFile(f)
		if err != nil {
			return fail(err)
		}
		body = string(b)
	} else if title == "" && kind == "" && !hasFlag(args, "fix") {
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
		fmt.Printf("  hash   %s… -> %s…\n", cur.Hash[:12], updated.Hash[:12])
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
	id, err := parseID(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: vestigio rm <id> [--yes]")
		return 2
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

	if !hasFlag(args, "yes") {
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
