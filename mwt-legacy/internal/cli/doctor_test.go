package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/cli"
	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/git"
	"github.com/Slothtron/mwt/internal/pathresolve"
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

// createEnvSetup writes .env into the worktree so doctor recheck can clear setup_missing.
type createEnvSetup struct {
	calls []string
	err   error
	errOn map[string]error
}

func (f *createEnvSetup) Run(ctx pathresolve.Context, steps []config.SetupStep) error {
	f.calls = append(f.calls, ctx.Repo)
	if f.errOn != nil {
		if err, ok := f.errOn[ctx.Repo]; ok {
			return err
		}
	}
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(filepath.Join(ctx.WorktreePath, ".env"), []byte("ok\n"), 0o644)
}

func TestDoctor_FixClearsSetupMissingAcrossRepos(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap")
	cfg.Setup = []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "{{MAIN_PATH}}/.env",
			To:   ".env",
		}},
	}

	lists := map[string][]git.Worktree{}
	for _, repo := range []string{"oauth", "sap"} {
		main := filepath.Join(root, repo)
		wt := filepath.Join(root, "worktrees", repo, "feat", repo)
		if err := os.MkdirAll(main, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(main, ".env"), []byte("src\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		lists[repo] = []git.Worktree{
			{Path: main, Branch: "master"},
			{Path: wt, Branch: "feat"},
		}
	}

	s := &createEnvSetup{}
	d := cli.Deps{
		Git:   &fakeGit{lists: lists},
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "doctor", "--fix")
	if err != nil {
		t.Fatalf("doctor --fix: %v\nstdout=%s", err, out)
	}
	if len(s.calls) != 2 || s.calls[0] != "oauth" || s.calls[1] != "sap" {
		t.Fatalf("setup calls=%v", s.calls)
	}
	if !strings.Contains(out, "fix: setup feat --repos oauth,sap") {
		t.Fatalf("missing fix progress: %q", out)
	}
	if !strings.Contains(out, "fix: cleared setup_missing") {
		t.Fatalf("missing cleared message: %q", out)
	}
	if !strings.Contains(out, "mwt doctor --fix") {
		t.Fatalf("report should suggest doctor --fix: %q", out)
	}
}

func TestDoctor_FixDoesNotRunSetupForPrunableOnly(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "gone", "feat")
	cfg := testCfg(t, root, "sap")
	s := &fakeSetup{}
	d := cli.Deps{
		Git: &fakeGit{lists: map[string][]git.Worktree{
			"sap": {
				{Path: main, Branch: "master"},
				{Path: stale, Branch: "feat"},
			},
		}},
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "doctor", "--fix")
	if err == nil {
		t.Fatal("expected non-zero with remaining prunable")
	}
	if len(s.calls) != 0 {
		t.Fatalf("setup must not run for prunable-only, calls=%v", s.calls)
	}
	if strings.Contains(out, "fix: setup") {
		t.Fatalf("should not print fix progress: %q", out)
	}
}

func TestDoctor_FixContinueAcrossRepos(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap")
	cfg.Setup = []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "{{MAIN_PATH}}/.env",
			To:   ".env",
		}},
	}

	lists := map[string][]git.Worktree{}
	for _, repo := range []string{"oauth", "sap"} {
		main := filepath.Join(root, repo)
		wt := filepath.Join(root, "worktrees", repo, "feat", repo)
		if err := os.MkdirAll(main, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(main, ".env"), []byte("src\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		lists[repo] = []git.Worktree{
			{Path: main, Branch: "master"},
			{Path: wt, Branch: "feat"},
		}
	}

	s := &createEnvSetup{errOn: map[string]error{
		"oauth": errors.New("setup boom"),
	}}
	d := cli.Deps{
		Git:   &fakeGit{lists: lists},
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "doctor", "--fix", "--continue")
	if err == nil {
		t.Fatal("expected aggregated setup error")
	}
	if len(s.calls) != 2 {
		t.Fatalf("continue should try both repos, calls=%v", s.calls)
	}
	if !strings.Contains(err.Error(), "oauth") {
		t.Fatalf("error should mention oauth: %v", err)
	}
	if !strings.Contains(out, "remaining after --fix:") {
		t.Fatalf("stdout=%q", out)
	}
}
