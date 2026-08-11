package setup_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/pathresolve"
	"github.com/Slothtron/mwt/internal/setup"
)

func TestRun_EmptySteps(t *testing.T) {
	ctx := testCtx(t)
	r := setup.New()
	if err := r.Run(ctx, nil); err != nil {
		t.Fatalf("Run nil: %v", err)
	}
	if err := r.Run(ctx, []config.SetupStep{}); err != nil {
		t.Fatalf("Run empty: %v", err)
	}
}

func TestRun_SkipChain(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "func_x", "sap")
	mustMkdirAll(t, main)
	mustMkdirAll(t, wt)

	// Main checkout has no .env; meta-root has .env.example (fallback).
	secret := "SECRET=do-not-log\n"
	writeFile(t, filepath.Join(root, ".env.example"), secret)

	ctx := pathresolve.Context{
		Root:         root,
		Repo:         "sap",
		RepoPath:     "sap",
		MainPath:     main,
		Branch:       "func_x",
		WorktreePath: wt,
		WorktreeName: "sap",
	}

	var logs bytes.Buffer
	r := &setup.Runner{Stdout: &logs, Stderr: &logs}

	steps := []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "{{MAIN_PATH}}/.env",
			To:   ".env",
			// defaults: skip_if_exists / skip_if_missing_src = true
		}},
		{Copy: &config.CopyAction{
			From: "{{ROOT}}/.env.example",
			To:   ".env",
		}},
		{Copy: &config.CopyAction{
			From: "{{ROOT}}/.env.example",
			To:   ".env", // already exists → skip
		}},
	}

	if err := r.Run(ctx, steps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(wt, ".env"))
	if err != nil {
		t.Fatalf("read worktree .env: %v", err)
	}
	if string(got) != secret {
		t.Fatalf(".env content = %q, want %q", got, secret)
	}
	if strings.Contains(logs.String(), "SECRET") || strings.Contains(logs.String(), "do-not-log") {
		t.Fatalf("logs must not contain env file contents: %q", logs.String())
	}
}

func TestRun_CopyFail_MissingSrc(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "b", "sap")
	mustMkdirAll(t, main)
	mustMkdirAll(t, wt)

	ctx := pathresolve.Context{
		Root:         root,
		Repo:         "sap",
		RepoPath:     "sap",
		MainPath:     main,
		Branch:       "b",
		WorktreePath: wt,
		WorktreeName: "sap",
	}

	skip := false
	r := setup.New()
	err := r.Run(ctx, []config.SetupStep{
		{Copy: &config.CopyAction{
			From:             "{{MAIN_PATH}}/.env",
			To:               ".env",
			SkipIfMissingSrc: &skip,
		}},
	})
	if err == nil {
		t.Fatal("expected copy failure when source missing and skip_if_missing_src=false")
	}
	if !strings.Contains(err.Error(), "setup: step 0:") {
		t.Fatalf("error should identify step: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(wt, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not exist after failed copy; stat: %v", statErr)
	}
}

func TestRun_CopyFail_StopsLaterSteps(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "wt")
	mustMkdirAll(t, main)
	mustMkdirAll(t, wt)
	marker := filepath.Join(wt, "ran.txt")

	ctx := pathresolve.Context{
		Root:         root,
		Repo:         "sap",
		RepoPath:     "sap",
		MainPath:     main,
		Branch:       "b",
		WorktreePath: wt,
		WorktreeName: "sap",
	}

	skip := false
	r := &setup.Runner{Stdout: io.Discard, Stderr: io.Discard}
	err := r.Run(ctx, []config.SetupStep{
		{Copy: &config.CopyAction{
			From:             filepath.Join(main, "missing.env"),
			To:               ".env",
			SkipIfMissingSrc: &skip,
		}},
		{Run: &config.RunAction{
			Command: "touch ran.txt",
		}},
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("later run step must not execute after copy failure")
	}
}

func TestRun_RunNonZero(t *testing.T) {
	ctx := testCtx(t)
	r := &setup.Runner{Stdout: io.Discard, Stderr: io.Discard}
	err := r.Run(ctx, []config.SetupStep{
		{Run: &config.RunAction{Command: "exit 42"}},
	})
	if err == nil {
		t.Fatal("expected non-zero run to fail")
	}
	if !strings.Contains(err.Error(), "exit 42") && !strings.Contains(err.Error(), "exit status 42") {
		t.Fatalf("error should mention command / exit status: %v", err)
	}
}

func TestRun_UnknownPlaceholderFails(t *testing.T) {
	ctx := testCtx(t)
	r := &setup.Runner{Stdout: io.Discard, Stderr: io.Discard}
	err := r.Run(ctx, []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "{{NOT_A_REAL_PLACEHOLDER}}",
			To:   ".env",
		}},
	})
	if err == nil {
		t.Fatal("expected expand failure for unknown placeholder")
	}
	if !strings.Contains(err.Error(), "setup: step 0:") {
		t.Fatalf("error should identify step: %v", err)
	}
	if !strings.Contains(err.Error(), "{{NOT_A_REAL_PLACEHOLDER}}") {
		t.Fatalf("error should mention placeholder: %v", err)
	}
}

func TestRun_RunCommandUnknownPlaceholderFails(t *testing.T) {
	ctx := testCtx(t)
	r := &setup.Runner{Stdout: io.Discard, Stderr: io.Discard}
	err := r.Run(ctx, []config.SetupStep{
		{Run: &config.RunAction{Command: "echo {{BOGUS}}"}},
	})
	if err == nil {
		t.Fatal("expected expand failure for unknown placeholder in run.command")
	}
	if !strings.Contains(err.Error(), "{{BOGUS}}") {
		t.Fatalf("error should mention placeholder: %v", err)
	}
}

func TestRun_RunDefaultCwdIsWorktree(t *testing.T) {
	ctx := testCtx(t)
	r := &setup.Runner{Stdout: io.Discard, Stderr: io.Discard}
	err := r.Run(ctx, []config.SetupStep{
		{Run: &config.RunAction{Command: "pwd > cwd.txt"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(ctx.WorktreePath, "cwd.txt"))
	if err != nil {
		t.Fatalf("read cwd.txt: %v", err)
	}
	gotPath := strings.TrimSpace(string(got))
	want, err := filepath.EvalSymlinks(ctx.WorktreePath)
	if err != nil {
		want = ctx.WorktreePath
	}
	gotResolved, err := filepath.EvalSymlinks(gotPath)
	if err != nil {
		gotResolved = gotPath
	}
	if gotResolved != want {
		t.Fatalf("cwd = %q, want %q", gotResolved, want)
	}
}

func TestRun_RelativePaths(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "oauth")
	wt := filepath.Join(root, "worktrees", "oauth", "b", "oauth")
	mustMkdirAll(t, main)
	mustMkdirAll(t, wt)
	writeFile(t, filepath.Join(root, "templates", "env"), "FROM_ROOT=1\n")

	ctx := pathresolve.Context{
		Root:         root,
		Repo:         "oauth",
		RepoPath:     "oauth",
		MainPath:     main,
		Branch:       "b",
		WorktreePath: wt,
		WorktreeName: "oauth",
	}

	r := setup.New()
	err := r.Run(ctx, []config.SetupStep{
		{Copy: &config.CopyAction{
			From: "templates/env", // relative → ROOT
			To:   "cfg/app.env",  // relative → WORKTREE_PATH
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(wt, "cfg", "app.env"))
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "FROM_ROOT=1\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestRun_AbsolutePathsUnchanged(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "wt")
	other := t.TempDir()
	mustMkdirAll(t, main)
	mustMkdirAll(t, wt)

	src := filepath.Join(other, "src.env")
	dst := filepath.Join(other, "dst.env")
	writeFile(t, src, "ABS=1\n")

	ctx := pathresolve.Context{
		Root:         root,
		Repo:         "sap",
		RepoPath:     "sap",
		MainPath:     main,
		Branch:       "b",
		WorktreePath: wt,
		WorktreeName: "sap",
	}

	r := setup.New()
	err := r.Run(ctx, []config.SetupStep{
		{Copy: &config.CopyAction{From: src, To: dst}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read abs dest: %v", err)
	}
	if string(got) != "ABS=1\n" {
		t.Fatalf("content = %q", got)
	}
}

func testCtx(t *testing.T) pathresolve.Context {
	t.Helper()
	root := t.TempDir()
	main := filepath.Join(root, "sap")
	wt := filepath.Join(root, "worktrees", "sap", "b", "sap")
	mustMkdirAll(t, main)
	mustMkdirAll(t, wt)
	return pathresolve.Context{
		Root:         root,
		Repo:         "sap",
		RepoPath:     "sap",
		MainPath:     main,
		Branch:       "b",
		WorktreePath: wt,
		WorktreeName: "sap",
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
