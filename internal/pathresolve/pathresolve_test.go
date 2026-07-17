package pathresolve_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

func TestResolve_SyncAuthStyle(t *testing.T) {
	metaRoot := "/home/u/sync-auth"
	tmpl := "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"

	ctx, err := pathresolve.Resolve(metaRoot, tmpl, "sap", "func_x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	wantPath := filepath.Clean("/home/u/sync-auth/worktrees/sap/func_x/sap")
	if ctx.WorktreePath != wantPath {
		t.Fatalf("WorktreePath = %q, want %q", ctx.WorktreePath, wantPath)
	}
	if ctx.WorktreeName != "sap" {
		t.Fatalf("WorktreeName = %q, want %q", ctx.WorktreeName, "sap")
	}
	if ctx.Root != filepath.Clean(metaRoot) {
		t.Fatalf("Root = %q, want %q", ctx.Root, filepath.Clean(metaRoot))
	}
	if ctx.Repo != "sap" || ctx.RepoPath != "sap" {
		t.Fatalf("Repo/RepoPath = %q/%q, want sap/sap", ctx.Repo, ctx.RepoPath)
	}
	wantMain := filepath.Join(metaRoot, "sap")
	if ctx.MainPath != wantMain {
		t.Fatalf("MainPath = %q, want %q", ctx.MainPath, wantMain)
	}
	if ctx.Branch != "func_x" {
		t.Fatalf("Branch = %q, want func_x", ctx.Branch)
	}
}

func TestResolve_RelativeToAbsolute(t *testing.T) {
	metaRoot := "/tmp/meta"
	tmpl := ".worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"

	ctx, err := pathresolve.Resolve(metaRoot, tmpl, "oauth", "feat")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !filepath.IsAbs(ctx.WorktreePath) {
		t.Fatalf("WorktreePath is not absolute: %q", ctx.WorktreePath)
	}
	want := filepath.Clean("/tmp/meta/.worktrees/oauth/feat/oauth")
	if ctx.WorktreePath != want {
		t.Fatalf("WorktreePath = %q, want %q", ctx.WorktreePath, want)
	}
}

func TestResolve_AbsoluteTemplateUnchangedPrefix(t *testing.T) {
	metaRoot := "/home/u/sync-auth"
	tmpl := "/var/wt/{{REPO}}/{{BRANCH}}"

	ctx, err := pathresolve.Resolve(metaRoot, tmpl, "sap", "func_x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Clean("/var/wt/sap/func_x")
	if ctx.WorktreePath != want {
		t.Fatalf("WorktreePath = %q, want %q", ctx.WorktreePath, want)
	}
}

func TestResolve_UnknownPlaceholder(t *testing.T) {
	_, err := pathresolve.Resolve("/meta", "worktrees/{{UNKNOWN}}/{{BRANCH}}", "sap", "b")
	if err == nil {
		t.Fatal("expected error for unknown placeholder")
	}
	if !strings.Contains(err.Error(), "{{UNKNOWN}}") {
		t.Fatalf("error should mention unknown placeholder: %v", err)
	}
}

func TestResolve_SelfRefForbidden_WorktreePath(t *testing.T) {
	_, err := pathresolve.Resolve(
		"/meta",
		"{{WORKTREE_PATH}}/extra",
		"sap",
		"b",
	)
	if err == nil {
		t.Fatal("expected error for WORKTREE_PATH self-ref")
	}
	if !strings.Contains(err.Error(), "WORKTREE_PATH") {
		t.Fatalf("error should mention WORKTREE_PATH: %v", err)
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error should say forbidden: %v", err)
	}
}

func TestResolve_SelfRefForbidden_WorktreeName(t *testing.T) {
	_, err := pathresolve.Resolve(
		"/meta",
		"worktrees/{{REPO}}/{{WORKTREE_NAME}}",
		"sap",
		"b",
	)
	if err == nil {
		t.Fatal("expected error for WORKTREE_NAME self-ref")
	}
	if !strings.Contains(err.Error(), "WORKTREE_NAME") {
		t.Fatalf("error should mention WORKTREE_NAME: %v", err)
	}
}

func TestExpand_SetupStageAllPlaceholders(t *testing.T) {
	ctx, err := pathresolve.Resolve(
		"/home/u/sync-auth",
		"worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}",
		"sap",
		"func_x",
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got, err := ctx.Expand(
		"cp {{MAIN_PATH}}/.env {{WORKTREE_PATH}}/.env # {{WORKTREE_NAME}} @ {{ROOT}}",
		pathresolve.StageSetup,
	)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := "cp /home/u/sync-auth/sap/.env /home/u/sync-auth/worktrees/sap/func_x/sap/.env # sap @ /home/u/sync-auth"
	if got != want {
		t.Fatalf("Expand = %q, want %q", got, want)
	}
}

func TestExpand_SetupUnknownPlaceholder(t *testing.T) {
	ctx, err := pathresolve.Resolve("/meta", "worktrees/{{REPO}}/{{BRANCH}}", "r", "b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	_, err = ctx.Expand("{{FOO}}", pathresolve.StageSetup)
	if err == nil {
		t.Fatal("expected error for unknown placeholder in setup")
	}
}

func TestResolveFromConfig_UsesTemplateAsIs(t *testing.T) {
	cfg := &config.Config{
		MetaRoot:     "/home/u/proj",
		WorktreePath: "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}",
		Repos:        []string{"oauth"},
	}
	ctx, err := pathresolve.ResolveFromConfig(cfg, "oauth", "feat")
	if err != nil {
		t.Fatalf("ResolveFromConfig: %v", err)
	}
	want := filepath.Clean("/home/u/proj/worktrees/oauth/feat/oauth")
	if ctx.WorktreePath != want {
		t.Fatalf("WorktreePath = %q, want %q", ctx.WorktreePath, want)
	}
}

func TestResolve_UsesREPO_PATHAndMAIN_PATH(t *testing.T) {
	ctx, err := pathresolve.Resolve(
		"/meta",
		"{{MAIN_PATH}}/../wt/{{REPO_PATH}}/{{BRANCH}}",
		"sap",
		"b1",
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Clean collapses ../ 
	want := filepath.Clean("/meta/wt/sap/b1")
	if ctx.WorktreePath != want {
		t.Fatalf("WorktreePath = %q, want %q", ctx.WorktreePath, want)
	}
}
