package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/cli"
	"github.com/Slothtron/mwt/internal/config"
)

func TestInit_WritesConfig_NoGitAtRoot(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, filepath.Join(root, "oauth"))
	mkGitDir(t, filepath.Join(root, "sap"))

	chdir(t, root)

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	data, err := os.ReadFile(filepath.Join(root, config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "repos:") {
		t.Fatalf("missing repos:\n%s", body)
	}
	if !strings.Contains(body, "- oauth") || !strings.Contains(body, "- sap") {
		t.Fatalf("missing repo entries:\n%s", body)
	}
	if !strings.Contains(body, "worktree_path: "+config.DefaultWorktreePathWithoutGit) {
		t.Fatalf("expected worktrees/ default:\n%s", body)
	}

	cfg, err := config.Load(filepath.Join(root, config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repos) != 2 {
		t.Fatalf("repos: %v", cfg.Repos)
	}
	if cfg.WorktreePath != config.DefaultWorktreePathWithoutGit {
		t.Fatalf("WorktreePath=%q", cfg.WorktreePath)
	}
}

func TestInit_WritesConfig_WithGitAtRoot(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root) // meta-root is itself a git repo; nested repos not scanned

	chdir(t, root)

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}

	data, err := os.ReadFile(filepath.Join(root, config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "worktree_path: "+config.DefaultWorktreePathWithGit) {
		t.Fatalf("expected .worktrees/ default:\n%s", body)
	}
	if !strings.Contains(body, "- .") {
		t.Fatalf("expected repos: [.] :\n%s", body)
	}

	cfg, err := config.Load(filepath.Join(root, config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorktreePath != config.DefaultWorktreePathWithGit {
		t.Fatalf("WorktreePath=%q", cfg.WorktreePath)
	}
}

func TestInit_DryRunNoWrite(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, filepath.Join(root, "oauth"))
	chdir(t, root)

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"init", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, config.ConfigFileName)); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not write file, err=%v", err)
	}
	if !strings.Contains(out.String(), "would write") {
		t.Fatalf("dry-run output: %s", out.String())
	}
	if !strings.Contains(out.String(), "- oauth") {
		t.Fatalf("dry-run missing repos: %s", out.String())
	}
	if !strings.Contains(out.String(), "worktree_path:") {
		t.Fatalf("dry-run missing worktree_path: %s", out.String())
	}
}

func TestInit_RefuseExistingWithoutForce(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, filepath.Join(root, "oauth"))
	if err := os.WriteFile(filepath.Join(root, config.ConfigFileName), []byte("repos: [old]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)

	cmd := cli.NewRootCmd()
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when .mwt.yaml exists")
	}

	cmd = cli.NewRootCmd()
	cmd.SetArgs([]string{"init", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "oauth") {
		t.Fatalf("force overwrite failed: %s", data)
	}
	if !strings.Contains(string(data), "worktree_path:") {
		t.Fatalf("force rewrite missing worktree_path: %s", data)
	}
}

func TestInit_NoRepos(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	cmd := cli.NewRootCmd()
	cmd.SetArgs([]string{"init"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when no git repos found")
	}
}

func mkGitDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}
