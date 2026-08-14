package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The regression these tests exist for: "--map old=new" once parsed as a bare
// "--map" that matched nothing, followed by an "old=new" that looked like a
// positional and overwrote the export path. Import ran against a file named
// "old=new" and failed with a message pointing nowhere near the real cause.
//
// The rule being pinned down is not "reject this one typo". It is that an
// argument the parser does not understand must stop the command, never fall
// through into something that happens to accept it.
func TestParseImportArgsRejectsSpaceSeparatedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"export.json", "--map", "old=new"},
		{"export.json", "--skip", "session"},
		{"--map", "old=new", "export.json"},
	} {
		path, _, err := parseImportArgs(args)
		if err == nil {
			t.Errorf("parseImportArgs(%q) = %q, nil; want an error — a bare flag must not be ignored", args, path)
		}
		if path != "" {
			t.Errorf("parseImportArgs(%q) returned path %q; a rejected parse must not hand back a path", args, path)
		}
	}
}

func TestParseImportArgsInlineFlags(t *testing.T) {
	path, opt, err := parseImportArgs([]string{
		"export.json",
		"--dry-run",
		"--map=Old-Repo= myrepo ,other=myrepo",
		"--skip=session,log",
	})
	if err != nil {
		t.Fatalf("parseImportArgs: unexpected error: %v", err)
	}
	if path != "export.json" {
		t.Errorf("path = %q, want %q", path, "export.json")
	}
	if !opt.DryRun {
		t.Error("DryRun = false, want true")
	}
	// Both sides of a mapping are lowercased and trimmed.
	if got := opt.ProjectMap["old-repo"]; got != "myrepo" {
		t.Errorf("ProjectMap[old-repo] = %q, want %q", got, "myrepo")
	}
	if got := opt.ProjectMap["other"]; got != "myrepo" {
		t.Errorf("ProjectMap[other] = %q, want %q", got, "myrepo")
	}
	if !opt.SkipTypes["session"] || !opt.SkipTypes["log"] {
		t.Errorf("SkipTypes = %v, want session and log", opt.SkipTypes)
	}
}

func TestParseImportArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"export.json", "--verbose"}},
		{"single dash flag", []string{"export.json", "-n"}},
		{"no path", []string{"--dry-run"}},
		{"only flags", []string{"--map=a=b", "--skip=x"}},
		// A second positional is far more likely to be a mistyped flag value
		// than a second export, and silently keeping one of the two is how the
		// original bug did its damage.
		{"second positional", []string{"one.json", "two.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := parseImportArgs(tt.args); err == nil {
				t.Errorf("parseImportArgs(%q) = nil error, want an error", tt.args)
			}
		})
	}
}

// scratchDir moves the test into an empty directory with a name we choose, and
// verifies git finds no remote from there. The verification is the point: every
// test below distinguishes "detection found nothing" from "detection found
// something", so a temp directory that turned out to sit inside a repository
// would not fail these tests, it would make them lie.
func scratchDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(dir)
	if out, err := exec.Command("git", "config", "--get", "remote.origin.url").Output(); err == nil && len(out) > 0 {
		t.Skipf("temp dir sits inside a git repository with remote %q", out)
	}
	return dir
}

// repoDir builds a real repository with a real remote. Nothing is committed —
// `git config --get remote.origin.url` only reads config, and a fixture that
// does the least is a fixture that keeps passing.
func repoDir(t *testing.T, remote string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", remote},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestProjectOverrideBeatsDetection(t *testing.T) {
	repoDir(t, "https://github.com/valzkat1/vestigio.git")
	t.Setenv("VESTIGIO_PROJECT", "pinned")
	t.Setenv("VESTIGIO_DEFAULT_PROJECT", "fallback")

	if got := detectProject(); got != "pinned" {
		t.Errorf("detectProject() = %q, want %q — VESTIGIO_PROJECT is an override and must win", got, "pinned")
	}
}

// The reason VESTIGIO_DEFAULT_PROJECT exists at all. VESTIGIO_PROJECT could
// already rescue a client launched outside any repository, but it wins
// everywhere, so setting it in a config shared across directories silently
// destroys per-repository scoping. A default has to LOSE here, or it is just a
// second override with a longer name.
func TestDefaultProjectLosesToTheGitRemote(t *testing.T) {
	repoDir(t, "git@github.com:valzkat1/AlCubo.git")
	t.Setenv("VESTIGIO_PROJECT", "")
	t.Setenv("VESTIGIO_DEFAULT_PROJECT", "personal")

	if got := detectProject(); got != "alcubo" {
		t.Errorf("detectProject() = %q, want %q — a default must never outrank a detected remote", got, "alcubo")
	}
}

// The Codex Desktop bug, reproduced. That client launches the server in
// Documents/Codex/<date>/<name>, which is not a repository, so the directory
// name became the project — and since the date is in the path, a different
// project every day.
func TestDefaultProjectCatchesTheScratchDirectory(t *testing.T) {
	scratchDir(t, "ho")
	t.Setenv("VESTIGIO_PROJECT", "")

	t.Setenv("VESTIGIO_DEFAULT_PROJECT", "")
	if got := detectProject(); got != "ho" {
		t.Fatalf("detectProject() = %q, want %q — the fixture is not reproducing the bug it exists for", got, "ho")
	}

	t.Setenv("VESTIGIO_DEFAULT_PROJECT", "personal")
	if got := detectProject(); got != "personal" {
		t.Errorf("detectProject() = %q, want %q — the default must replace the directory-name guess", got, "personal")
	}
}

func TestDirectoryNameIsTheLastResort(t *testing.T) {
	scratchDir(t, "MyScratchDir")
	t.Setenv("VESTIGIO_PROJECT", "")
	t.Setenv("VESTIGIO_DEFAULT_PROJECT", "")

	if got := detectProject(); got != "myscratchdir" {
		t.Errorf("detectProject() = %q, want %q", got, "myscratchdir")
	}
}

func TestRepoFromRemote(t *testing.T) {
	tests := map[string]string{
		"git@github.com:valzkat1/AlCubo.git":      "alcubo",
		"https://github.com/valzkat1/vestigio":    "vestigio",
		"https://github.com/valzkat1/Repo/":       "repo",
		"ssh://git@host.example/team/Service.git": "service",
		"": "",
	}
	for remote, want := range tests {
		if got := repoFromRemote(remote); got != want {
			t.Errorf("repoFromRemote(%q) = %q, want %q", remote, got, want)
		}
	}
}
