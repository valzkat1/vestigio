package main

import "testing"

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
