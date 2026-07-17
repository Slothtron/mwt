package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/cli"
	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/git"
)

func TestDoctor_ReportsPrunableAndExitsNonZero(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "gone", "feat")
	cfg := testCfg(t, root, "sap")
	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: stale, Branch: "feat"},
		},
	}}
	d := cli.Deps{
		Git: g,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "doctor")
	if err == nil {
		t.Fatal("doctor should exit non-zero when findings exist")
	}
	if !strings.Contains(out, "[prunable]") {
		t.Fatalf("stdout=%q", out)
	}
	if !strings.Contains(out, "worktree prune") || !strings.Contains(out, "mwt add feat --repos sap") {
		t.Fatalf("suggestions missing: %q", out)
	}
	canonical := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	if !strings.Contains(out, canonical) {
		t.Fatalf("canonical path missing: %q", out)
	}
}

func TestDoctor_OkWhenHealthy(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testCfg(t, root, "sap")
	g := &fakeGit{lists: map[string][]git.Worktree{
		"sap": {
			{Path: main, Branch: "master"},
			{Path: wt, Branch: "feat"},
		},
	}}
	d := cli.Deps{
		Git: g,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(out, "ok: no issues found") {
		t.Fatalf("stdout=%q", out)
	}
}
