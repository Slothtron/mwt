package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/cli"
	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/git"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

type fakeGit struct {
	adds    []string
	removes []string
	lists   map[string][]git.Worktree
	addErr  map[string]error
	rmErr   map[string]error
	listErr map[string]error
}

func (f *fakeGit) Add(repoPath, worktreePath, branch, from string) error {
	key := filepath.Base(repoPath)
	f.adds = append(f.adds, fmt.Sprintf("%s|%s|%s|%s", key, worktreePath, branch, from))
	if f.addErr != nil {
		if err, ok := f.addErr[key]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeGit) Remove(repoPath, worktreePath string, force bool) error {
	key := filepath.Base(repoPath)
	f.removes = append(f.removes, fmt.Sprintf("%s|%s|%v", key, worktreePath, force))
	if f.rmErr != nil {
		if err, ok := f.rmErr[key]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeGit) List(repoPath string) ([]git.Worktree, error) {
	key := filepath.Base(repoPath)
	if f.listErr != nil {
		if err, ok := f.listErr[key]; ok {
			return nil, err
		}
	}
	if f.lists == nil {
		return nil, nil
	}
	return f.lists[key], nil
}

type fakeSetup struct {
	calls []string
	err   error
}

func (f *fakeSetup) Run(ctx pathresolve.Context, steps []config.SetupStep) error {
	f.calls = append(f.calls, ctx.Repo)
	return f.err
}

func testCfg(t *testing.T, metaRoot string, repos ...string) *config.Config {
	t.Helper()
	return &config.Config{
		Root:         ".",
		WorktreePath: "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}",
		Repos:         repos,
		MetaRoot:     metaRoot,
		ConfigPath:   filepath.Join(metaRoot, ".mwt.yaml"),
	}
}

func execCmd(t *testing.T, d cli.Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	d.Stdout = &out
	d.Stderr = &errBuf
	cmd := cli.NewRootCmdWith(d)
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	err = cmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestAdd_CreatesAndRunsSetup(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap")
	g := &fakeGit{}
	s := &fakeSetup{}
	var mkdirs []string

	d := cli.Deps{
		Git:   g,
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: func(path string, perm os.FileMode) error {
			mkdirs = append(mkdirs, path)
			return nil
		},
	}

	out, _, err := execCmd(t, d, "add", "feat", "--from", "master")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(g.adds) != 2 {
		t.Fatalf("adds=%v", g.adds)
	}
	if len(s.calls) != 2 {
		t.Fatalf("setup calls=%v", s.calls)
	}
	if !strings.Contains(out, "oauth  worktrees/oauth/feat/oauth") || !strings.Contains(out, "sap    worktrees/sap/feat/sap") {
		t.Fatalf("stdout=%q", out)
	}
	if len(mkdirs) != 2 {
		t.Fatalf("mkdirs=%v", mkdirs)
	}
	if !strings.HasSuffix(g.adds[0], "|feat|master") {
		t.Fatalf("from not passed: %v", g.adds[0])
	}
}

func TestAdd_NoSetup(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	g := &fakeGit{}
	s := &fakeSetup{}
	d := cli.Deps{
		Git:   g,
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
	}
	if _, _, err := execCmd(t, d, "add", "feat", "--no-setup"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(s.calls) != 0 {
		t.Fatalf("setup should be skipped, got %v", s.calls)
	}
}

func TestAdd_SetupFailure(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	g := &fakeGit{}
	s := &fakeSetup{err: errors.New("setup boom")}
	d := cli.Deps{
		Git:   g,
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
	}
	_, _, err := execCmd(t, d, "add", "feat")
	if err == nil {
		t.Fatal("expected setup failure to fail add")
	}
	if !strings.Contains(err.Error(), "setup boom") && !strings.Contains(err.Error(), "sap") {
		t.Fatalf("error should mention setup failure / repo: %v", err)
	}
	if len(g.adds) != 1 {
		t.Fatalf("git add should still have run before setup, adds=%v", g.adds)
	}
	if len(s.calls) != 1 {
		t.Fatalf("setup should have been attempted, calls=%v", s.calls)
	}
}

func TestAdd_ReposSubset(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap", "org-sync")
	g := &fakeGit{}
	d := cli.Deps{
		Git:   g,
		Setup: &fakeSetup{},
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
	}
	if _, _, err := execCmd(t, d, "add", "feat", "--repos", "sap", "--no-setup"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(g.adds) != 1 || !strings.HasPrefix(g.adds[0], "sap|") {
		t.Fatalf("adds=%v", g.adds)
	}
}

func TestAdd_StopOnFirstError(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap")
	g := &fakeGit{addErr: map[string]error{"oauth": errors.New("boom")}}
	d := cli.Deps{
		Git:   g,
		Setup: &fakeSetup{},
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
	}
	_, _, err := execCmd(t, d, "add", "feat", "--no-setup")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(g.adds) != 1 {
		t.Fatalf("should stop after first failure, adds=%v", g.adds)
	}
}

func TestAdd_ContinueAggregatesErrors(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap")
	g := &fakeGit{addErr: map[string]error{"oauth": errors.New("boom")}}
	d := cli.Deps{
		Git:   g,
		Setup: &fakeSetup{},
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
	}
	out, _, err := execCmd(t, d, "add", "feat", "--no-setup", "--continue")
	if err == nil {
		t.Fatal("expected non-zero / error after continue")
	}
	if !strings.Contains(err.Error(), "oauth") {
		t.Fatalf("error should mention oauth: %v", err)
	}
	if len(g.adds) != 2 {
		t.Fatalf("continue should try both, adds=%v", g.adds)
	}
	if !strings.Contains(out, "sap    worktrees/sap/feat/sap") {
		t.Fatalf("successful repo should still print, out=%q", out)
	}
}

func TestRm_Force(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	g := &fakeGit{}
	d := cli.Deps{
		Git: g,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "rm", "feat", "--force")
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if len(g.removes) != 1 || !strings.HasSuffix(g.removes[0], "|true") {
		t.Fatalf("removes=%v", g.removes)
	}
	if !strings.Contains(out, "sap  worktrees/sap/feat/sap") {
		t.Fatalf("out=%q", out)
	}
}

func TestList_BranchFilter(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap", "oauth")
	g := &fakeGit{
		lists: map[string][]git.Worktree{
			"sap": {
				{Path: filepath.Join(root, "worktrees", "sap", "a"), Branch: "feat"},
				{Path: filepath.Join(root, "worktrees", "sap", "b"), Branch: "other"},
			},
			"oauth": {
				{Path: filepath.Join(root, "worktrees", "oauth", "a"), Branch: "feat"},
			},
		},
	}
	d := cli.Deps{
		Git: g,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "list", "--branch", "feat")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(out, "other") {
		t.Fatalf("filter failed: %q", out)
	}
	if !strings.Contains(out, "REPO") || !strings.Contains(out, "BRANCH") || !strings.Contains(out, "PATH") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "sap    feat    worktrees/sap/a") || !strings.Contains(out, "oauth  feat    worktrees/oauth/a") {
		t.Fatalf("out=%q", out)
	}
}

func TestList_RepoFailureNonZero(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap", "oauth")
	g := &fakeGit{
		lists: map[string][]git.Worktree{
			"oauth": {{Path: filepath.Join(root, "worktrees", "oauth", "a"), Branch: "feat"}},
		},
		listErr: map[string]error{"sap": errors.New("missing")},
	}
	d := cli.Deps{
		Git: g,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "list", "--continue")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out, "oauth  feat    worktrees/oauth/a") {
		t.Fatalf("should still print successes: %q", out)
	}
}

func TestSetup_RequiresExistingWorktree(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	s := &fakeSetup{}
	d := cli.Deps{
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	_, _, err := execCmd(t, d, "setup", "feat")
	if err == nil {
		t.Fatal("expected missing worktree error")
	}
	if !strings.Contains(err.Error(), "worktree does not exist") {
		t.Fatalf("error=%v", err)
	}
	if len(s.calls) != 0 {
		t.Fatalf("setup must not run when dir missing, calls=%v", s.calls)
	}
}

func TestSetup_RunsOnExistingWorktree(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap")
	wtOAuth := filepath.Join(root, "worktrees", "oauth", "feat", "oauth")
	wtSap := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	if err := os.MkdirAll(wtOAuth, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtSap, 0o755); err != nil {
		t.Fatal(err)
	}

	s := &fakeSetup{}
	d := cli.Deps{
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "setup", "feat")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(s.calls) != 2 || s.calls[0] != "oauth" || s.calls[1] != "sap" {
		t.Fatalf("setup calls=%v", s.calls)
	}
	if !strings.Contains(out, "oauth  worktrees/oauth/feat/oauth") || !strings.Contains(out, "sap    worktrees/sap/feat/sap") {
		t.Fatalf("stdout=%q", out)
	}
}

func TestSetup_AfterAddNoSetup(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	g := &fakeGit{}
	s := &fakeSetup{}
	d := cli.Deps{
		Git:   g,
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
	}

	if _, _, err := execCmd(t, d, "add", "feat", "--no-setup"); err != nil {
		t.Fatalf("add --no-setup: %v", err)
	}
	if len(s.calls) != 0 {
		t.Fatalf("add --no-setup should skip setup, got %v", s.calls)
	}

	// add creates via git adapter only; materialize the resolved path for setup.
	wt := filepath.Join(root, "worktrees", "sap", "feat", "sap")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}

	out, _, err := execCmd(t, d, "setup", "feat")
	if err != nil {
		t.Fatalf("setup after add: %v", err)
	}
	if len(s.calls) != 1 || s.calls[0] != "sap" {
		t.Fatalf("setup calls=%v", s.calls)
	}
	if !strings.Contains(out, "sap  worktrees/sap/feat/sap") {
		t.Fatalf("stdout=%q", out)
	}
}

func TestSetup_ReposSubsetAndContinue(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "oauth", "sap", "org-sync")
	for _, repo := range []string{"oauth", "sap"} {
		wt := filepath.Join(root, "worktrees", repo, "feat", repo)
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	s := &fakeSetup{err: errors.New("setup boom")}
	d := cli.Deps{
		Setup: s,
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	_, _, err := execCmd(t, d, "setup", "feat", "--repos", "oauth,sap", "--continue")
	if err == nil {
		t.Fatal("expected aggregated setup error")
	}
	if !strings.Contains(err.Error(), "oauth") || !strings.Contains(err.Error(), "sap") {
		t.Fatalf("error should mention both repos: %v", err)
	}
	if len(s.calls) != 2 {
		t.Fatalf("continue should try both, calls=%v", s.calls)
	}
}

func TestPath_PrintsAbsoluteOnly(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	d := cli.Deps{
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "path", "feat", "sap")
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	want := filepath.Join(root, "worktrees", "sap", "feat", "sap") + "\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestPath_UnknownRepo(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	d := cli.Deps{
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	_, _, err := execCmd(t, d, "path", "feat", "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSelectReposViaUnknownFlag(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(t, root, "sap")
	d := cli.Deps{
		Git: &fakeGit{},
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		MkdirAll: os.MkdirAll,
		Setup:    &fakeSetup{},
	}
	_, _, err := execCmd(t, d, "add", "feat", "--repos", "missing", "--no-setup")
	if err == nil {
		t.Fatal("expected unknown repo error")
	}
}

func TestDualDefaultPathViaConfig(t *testing.T) {
	root := t.TempDir()
	// Simulate config.Load dual default: with .git → .worktrees/
	cfg := &config.Config{
		WorktreePath: config.DefaultWorktreePathWithGit,
		Repos:         []string{"sap"},
		MetaRoot:     root,
		HasGitAtRoot: true,
	}
	d := cli.Deps{
		LoadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
	out, _, err := execCmd(t, d, "path", "feat", "sap")
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	want := filepath.Join(root, ".worktrees", "sap", "feat", "sap") + "\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}
