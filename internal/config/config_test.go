package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

func TestLoad_DefaultWorktreePath_WithGitDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
repos:
  - oauth
`)
	// Directory .git
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.WorktreePath, config.DefaultWorktreePathWithGit; got != want {
		t.Fatalf("WorktreePath = %q, want %q", got, want)
	}
	if !cfg.HasGitAtRoot {
		t.Fatal("HasGitAtRoot = false, want true")
	}
}

func TestLoad_DefaultWorktreePath_WithGitFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
repos: [sap]
`)
	// gitfile (linked worktree style)
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /tmp/somewhere\n")

	cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.WorktreePath, config.DefaultWorktreePathWithGit; got != want {
		t.Fatalf("WorktreePath = %q, want %q", got, want)
	}
	if !cfg.HasGitAtRoot {
		t.Fatal("HasGitAtRoot = false, want true")
	}
}

func TestLoad_DefaultWorktreePath_WithoutGit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
repos:
  - oauth
  - sap
`)

	cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.WorktreePath, config.DefaultWorktreePathWithoutGit; got != want {
		t.Fatalf("WorktreePath = %q, want %q", got, want)
	}
	if cfg.HasGitAtRoot {
		t.Fatal("HasGitAtRoot = true, want false")
	}
}

func TestLoad_ExplicitWorktreePath_NotRewritten_WithGit(t *testing.T) {
	root := t.TempDir()
	explicit := "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
worktree_path: "`+explicit+`"
repos: [oauth]
`)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WorktreePath != explicit {
		t.Fatalf("WorktreePath = %q, want explicit %q (must not rewrite to .worktrees)", cfg.WorktreePath, explicit)
	}
}

func TestLoad_ExplicitWorktreePath_NotRewritten_WithoutGit(t *testing.T) {
	root := t.TempDir()
	explicit := ".worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
worktree_path: "`+explicit+`"
repos: [oauth]
`)

	cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WorktreePath != explicit {
		t.Fatalf("WorktreePath = %q, want explicit %q (must not rewrite to worktrees)", cfg.WorktreePath, explicit)
	}
}

func TestLoad_ParsesSetupAndRepos(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
root: .
worktree_path: "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
repos:
  - oauth
  - org-sync
  - sap
setup:
  - copy:
      from: "{{MAIN_PATH}}/.env"
      to: ".env"
      skip_if_exists: true
      skip_if_missing_src: true
  - run:
      command: "go mod download"
`)

	cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Repos) != 3 {
		t.Fatalf("repos len = %d, want 3", len(cfg.Repos))
	}
	if len(cfg.Setup) != 2 {
		t.Fatalf("setup len = %d, want 2", len(cfg.Setup))
	}
	if cfg.Setup[0].Copy == nil || cfg.Setup[0].Copy.From != "{{MAIN_PATH}}/.env" {
		t.Fatalf("setup[0] copy unexpected: %+v", cfg.Setup[0].Copy)
	}
	if cfg.Setup[1].Run == nil || cfg.Setup[1].Run.Command != "go mod download" {
		t.Fatalf("setup[1] run unexpected: %+v", cfg.Setup[1].Run)
	}
}

func TestLoad_UnknownSetupAction(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
repos: [oauth]
setup:
  - mkdir:
      path: foo
`)

	_, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err == nil {
		t.Fatal("Load: want error for unknown setup action")
	}
}

func TestFindConfigFile_WalksUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `repos: [a]`)
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := config.FindConfigFile(nested)
	if err != nil {
		t.Fatalf("FindConfigFile: %v", err)
	}
	want := filepath.Join(root, ".mwt.yaml")
	if got != want {
		t.Fatalf("FindConfigFile = %q, want %q", got, want)
	}
}

func TestLoadFromDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `repos: [a]`)
	nested := filepath.Join(root, "x")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(nested)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if cfg.MetaRoot != root {
		t.Fatalf("MetaRoot = %q, want %q", cfg.MetaRoot, root)
	}
}

// T08: Load → ResolveFromConfig end-to-end dual-default / explicit path prefixes.
func TestLoadAndResolve_DualDefaultAndExplicit(t *testing.T) {
	cases := []struct {
		name     string
		withGit  bool
		yamlBody string
		wantRel  string
	}{
		{
			name:     "with_git_default_dot_worktrees",
			withGit:  true,
			yamlBody: "repos: [sap]\n",
			wantRel:  filepath.Join(".worktrees", "sap", "feat", "sap"),
		},
		{
			name:     "without_git_default_worktrees",
			withGit:  false,
			yamlBody: "repos: [sap]\n",
			wantRel:  filepath.Join("worktrees", "sap", "feat", "sap"),
		},
		{
			name:    "explicit_not_rewritten_with_git",
			withGit: true,
			yamlBody: `worktree_path: "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
repos: [sap]
`,
			wantRel: filepath.Join("worktrees", "sap", "feat", "sap"),
		},
		{
			name:    "explicit_not_rewritten_without_git",
			withGit: false,
			yamlBody: `worktree_path: ".worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
repos: [sap]
`,
			wantRel: filepath.Join(".worktrees", "sap", "feat", "sap"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".mwt.yaml"), tc.yamlBody)
			if tc.withGit {
				if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			cfg, err := config.Load(filepath.Join(root, ".mwt.yaml"))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			ctx, err := pathresolve.ResolveFromConfig(cfg, "sap", "feat")
			if err != nil {
				t.Fatalf("ResolveFromConfig: %v", err)
			}
			want := filepath.Join(root, tc.wantRel)
			if ctx.WorktreePath != want {
				t.Fatalf("WorktreePath = %q, want %q", ctx.WorktreePath, want)
			}
			if strings.Contains(cfg.WorktreePath, "{{UNKNOWN}}") {
				t.Fatal("template should not contain unknown placeholders")
			}
		})
	}
}

func TestLoad_ValidateEmptyRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".mwt.yaml"), `
repos:
  - ""
`)
	_, err := config.Load(filepath.Join(root, ".mwt.yaml"))
	if err == nil {
		t.Fatal("Load: want error for empty repo name")
	}
	if !strings.Contains(err.Error(), "repos[0]") {
		t.Fatalf("error should mention repos[0]: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
