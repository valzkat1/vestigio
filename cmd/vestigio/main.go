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

func runMCP(args []string) int {
	project := ""
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "--project="); ok {
			project = v
		}
	}
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

// runImport migrates an Engram JSON export. Import is a CLI command and not an
// MCP tool on purpose: an agent never needs to call it, so it costs no context.
func runImport(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: vestigio import <export.json> [--dry-run] [--map old=new,...] [--skip type,...]")
		return 2
	}

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
		case !strings.HasPrefix(a, "--"):
			path = a
		}
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "vestigio: no export file given")
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

Tools exposed over MCP: recall, remember, forget.
Operational commands stay out of MCP on purpose — every tool exposed to an
agent is context paid for in every session.

Environment:
  VESTIGIO_DB        Database path (default ~/.vestigio/vestigio.db)
  VESTIGIO_PROJECT   Override project detection
`)
}
