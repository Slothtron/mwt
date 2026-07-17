package initscan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_PolyrepoNoTopGit(t *testing.T) {
	root := t.TempDir()
	mkGit(t, filepath.Join(root, "oauth"))
	mkGit(t, filepath.Join(root, "org-sync"))
	mkGit(t, filepath.Join(root, "nested", "sap"))

	repos, err := Discover(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"nested/sap", "oauth", "org-sync"}
	assertRepos(t, repos, want)
}

func TestDiscover_CwdIsGitRepo(t *testing.T) {
	root := t.TempDir()
	mkGit(t, root)
	// Nested git under a git root must NOT be discovered (no descend).
	mkGit(t, filepath.Join(root, "nested"))

	repos, err := Discover(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, repos, []string{"."})
}

func TestDiscover_SkipWorktreesDirs(t *testing.T) {
	root := t.TempDir()
	mkGit(t, filepath.Join(root, "oauth"))
	mkGit(t, filepath.Join(root, "worktrees", "oauth", "feat", "oauth"))
	mkGit(t, filepath.Join(root, ".worktrees", "oauth", "feat", "oauth"))

	repos, err := Discover(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, repos, []string{"oauth"})
}

func TestDiscover_DepthTruncation(t *testing.T) {
	root := t.TempDir()
	mkGit(t, filepath.Join(root, "a", "b", "c"))

	repos, err := Discover(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("depth=2 should not reach a/b/c, got %v", repos)
	}

	repos, err = Discover(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, repos, []string{"a/b/c"})
}

func TestDiscover_DepthZeroOnlyRoot(t *testing.T) {
	root := t.TempDir()
	mkGit(t, filepath.Join(root, "child"))

	repos, err := Discover(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("depth=0 and root not git: want empty, got %v", repos)
	}

	mkGit(t, root)
	repos, err = Discover(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, repos, []string{"."})
}

func TestDiscover_Gitfile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "linked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /tmp/fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos, err := Discover(root, 5)
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, repos, []string{"linked"})
}

func TestDiscover_NegativeDepth(t *testing.T) {
	_, err := Discover(t.TempDir(), -1)
	if err == nil {
		t.Fatal("expected error for negative depth")
	}
}

func TestDiscover_Sorted(t *testing.T) {
	root := t.TempDir()
	mkGit(t, filepath.Join(root, "z"))
	mkGit(t, filepath.Join(root, "a"))
	mkGit(t, filepath.Join(root, "m"))

	repos, err := Discover(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, repos, []string{"a", "m", "z"})
}

func mkGit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertRepos(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("repos len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("repos[%d]: got %q want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}
