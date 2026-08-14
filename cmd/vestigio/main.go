package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/valzkat1/vestigio/internal/importer"
	"github.com/valzkat1/vestigio/internal/mcp"
	"github.com/valzkat1/vestigio/internal/store"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	switch args[0] {
	case "mcp":
		os.Exit(runMCP(args[1:]))
	case "import":
		os.Exit(runImport(args[1:]))
	case "seed":
		os.Exit(runSeed(args[1:]))
	case "projects":
		os.Exit(runProjects(args[1:]))
	case "list", "ls":
		os.Exit(runList(args[1:]))
	case "show", "cat":
		os.Exit(runShow(args[1:]))
	case "edit":
		os.Exit(runEdit(args[1:]))
	case "rm", "delete":
		os.Exit(runRm(args[1:]))
	case "verify":
		os.Exit(runVerify(args[1:]))
	case "version", "--version", "-v":
		fmt.Println("vestigio", mcp.Version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		usage()
		os.Exit(2)
	}
}

var mcpSpec = argSpec{usage: "usage: vestigio mcp [--project=NAME]", value: []string{"project"}}

func runMCP(args []string) int {
	// A mistyped --porject used to be ignored, and the server would then serve a
	// different project than the one asked for. That is the failure detectProject
	// warns about: recall comes back empty and reads like data loss.
	a, err := parseArgs(args, mcpSpec)
	if err != nil {
		return usageErr(err)
	}
	project, _ := a.value("project")
	if project == "" {
		project = detectProject()
	}

	st, err := store.Open(store.DefaultPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "vestigio:", err)
		return 1
	}
	defer st.Close()

	// stdout is reserved for JSON-RPC frames — this line must go to stderr.
	fmt.Fprintf(os.Stderr, "vestigio %s — project %q\n", mcp.Version, project)

	if err := mcp.Serve(st, project, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "vestigio:", err)
		return 1
	}
	return 0
}

const importUsage = "usage: vestigio import <export.json> [--dry-run] [--map=old=new,...] [--skip=type,...]"

// parseImportArgs reads the import flags. Every flag carries its value inline,
// after "=".
//
// An unrecognised argument is an error rather than something to skip past, and
// that is the whole point of this function existing separately. The previous
// parser ignored anything it did not recognise, so "--map old=new" split into a
// bare "--map" that matched no case and an "old=new" that looked like a
// positional — which silently replaced the export path. Import then ran against
// a file named "old=new" and reported a plain "not found", pointing nowhere near
// the flag that caused it. A wrong answer delivered quietly is the failure mode
// this whole project is built against.
func parseImportArgs(args []string) (string, importer.Options, error) {
	opt := importer.Options{ProjectMap: map[string]string{}, SkipTypes: map[string]bool{}}
	path := ""

	for _, a := range args {
		switch {
		case a == "--dry-run":
			opt.DryRun = true
		case strings.HasPrefix(a, "--map="):
			for _, pair := range strings.Split(strings.TrimPrefix(a, "--map="), ",") {
				if from, to, ok := strings.Cut(pair, "="); ok {
					opt.ProjectMap[strings.ToLower(strings.TrimSpace(from))] = strings.ToLower(strings.TrimSpace(to))
				}
			}
		case strings.HasPrefix(a, "--skip="):
			for _, t := range strings.Split(strings.TrimPrefix(a, "--skip="), ",") {
				opt.SkipTypes[strings.TrimSpace(t)] = true
			}
		case a == "--map" || a == "--skip":
			return "", opt, fmt.Errorf("%s takes its value inline: %s=VALUE", a, a)
		case strings.HasPrefix(a, "-"):
			return "", opt, fmt.Errorf("unknown flag %q\n%s", a, importUsage)
		case path != "":
			return "", opt, fmt.Errorf("unexpected argument %q — the export path is already %q", a, path)
		default:
			path = a
		}
	}

	if path == "" {
		return "", opt, fmt.Errorf("no export file given\n%s", importUsage)
	}
	return path, opt, nil
}

// runImport migrates an Engram JSON export. Import is a CLI command and not an
// MCP tool on purpose: an agent never needs to call it, so it costs no context.
func runImport(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, importUsage)
		return 2
	}

	path, opt, err := parseImportArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "vestigio:", err)
		return 2
	}

	exp, err := importer.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "vestigio:", err)
		return 1
	}

	st, err := store.Open(store.DefaultPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "vestigio:", err)
		return 1
	}
	defer st.Close()

	if opt.DryRun {
		fmt.Println("DRY RUN — no se escribe nada")
	}
	fmt.Print(importer.Run(st, exp, opt).String())
	return 0
}

// detectProject prefers the git remote so that the same repository resolves to
// the same project from any clone path, and falls back to the directory name.
//
// M0 gotcha, learned the hard way: when detection silently picks the wrong
// project, every recall returns empty and it reads like data loss. Whatever this
// resolves to is printed on startup for exactly that reason.
func detectProject() string {
	if p := os.Getenv("VESTIGIO_PROJECT"); p != "" {
		return p
	}
	if out, err := exec.Command("git", "config", "--get", "remote.origin.url").Output(); err == nil {
		if name := repoFromRemote(strings.TrimSpace(string(out))); name != "" {
			return name
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return strings.ToLower(filepath.Base(cwd))
	}
	return "default"
}

func repoFromRemote(remote string) string {
	if remote == "" {
		return ""
	}
	remote = strings.TrimSuffix(remote, ".git")
	remote = strings.TrimSuffix(remote, "/")
	if i := strings.LastIndexAny(remote, "/:"); i >= 0 {
		remote = remote[i+1:]
	}
	return strings.ToLower(remote)
}

func usage() {
	fmt.Print(`vestigio — local memory for AI coding agents

Usage:
  vestigio mcp [--project=NAME]   Start the MCP server (stdio)
  vestigio version                Print version

Inspect and edit:
  vestigio projects               Inventory: memories and tokens per project
  vestigio list [flags]           List memories, newest first
       --project=NAME               default: detected project
       --all                        every project
       --kind=KIND                  decision|bugfix|pattern|constraint|reference
       --limit=N                    default 30
  vestigio show <id>              Print one memory in full
  vestigio edit <id> [flags]      Rewrite a memory, recomputing hash and tokens
       (no flags)                   open $VISUAL/$EDITOR on the body
       --title=TEXT --kind=KIND
       --body-file=FILE
       --fix                        recompute derived columns, keep content
  vestigio rm <id> [--yes]        Delete a memory
  vestigio verify                 Report rows whose hash or tokens drifted

Seed a new store from documents you already wrote:
  vestigio seed <file.md|file.txt> [flags]
       --project=NAME               default: detected project
       --kind=KIND                  fallback kind; the cascade wins over it
       --split=N                    cut markdown at heading level N (default: auto)
       --max-tokens=N               split sections larger than this (default 400)
       --dry-run                    print the plan, write nothing

       Markdown is cut on headings, plain text on "---" rules. Each section
       becomes one memory. Run --dry-run first: the cut is a guess about YOUR
       document, and it is cheaper to look than to clean up afterwards.

       Seed what was LEARNED — decision records, gotchas, post-mortems. Rules
       that were true before any code was written ("use strict mode") belong in
       AGENTS.md, which the agent already reads for free.

  Import an Engram export:
  vestigio import <export.json> [--dry-run] [--map=old=new,...] [--skip=type,...]
       Flag values are inline, after "=". A space-separated form is rejected
       rather than ignored, so it can never be mistaken for the export path.

Tools exposed over MCP: recall, remember, forget.
Operational commands stay out of MCP on purpose — every tool exposed to an
agent is context paid for in every session. They are still CLI commands: a
human browsing or repairing the store costs an agent nothing.

Never edit the database with a raw SQL prompt or a GUI browser. The FTS index
is kept in sync by triggers, but "hash" and "tokens" are computed on write and
no trigger touches them: a stale hash silently stops remember from
deduplicating, so it inserts near-copies instead of updating. "vestigio edit"
recomputes both; "vestigio verify" finds rows where that already happened.

Ids are global, not project-scoped — running from the wrong directory will
never hide a row that exists.

Environment:
  VESTIGIO_DB        Database path (default ~/.vestigio/vestigio.db)
  VESTIGIO_PROJECT   Override project detection
`)
}
