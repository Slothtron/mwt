// Package git adapts system git for multi-repo worktree operations (§3, §5.1).
//
// Core ops take absolute main-checkout paths and do not depend on pathresolve.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Adapter runs git worktree commands against per-repo main checkouts.
type Adapter struct {
	Runner Runner
}

// New returns an Adapter that shells out to system git via -C.
func New() *Adapter {
	return &Adapter{Runner: ExecRunner{}}
}

func (a *Adapter) runner() Runner {
	if a != nil && a.Runner != nil {
		return a.Runner
	}
	return ExecRunner{}
}

// Add creates a worktree at worktreePath for branch on the repo at repoPath.
//
// Flow (§3.1 / §3.3):
//  1. git worktree add <path> <branch>
//  2. if that fails, the branch is missing, and from is non-empty:
//     git worktree add -b <branch> <path> <from>
func (a *Adapter) Add(repoPath, worktreePath, branch, from string) error {
	if repoPath == "" {
		return fmt.Errorf("git add: repo path is empty")
	}
	if worktreePath == "" {
		return fmt.Errorf("git add: worktree path is empty (repo %s)", repoPath)
	}
	if branch == "" {
		return fmt.Errorf("git add: branch is empty (repo %s)", repoPath)
	}

	r := a.runner()
	_, err := r.Git(repoPath, "worktree", "add", worktreePath, branch)
	if err == nil {
		return nil
	}

	// Retry with -b only when branch is missing and a start point was provided.
	if from == "" {
		return err
	}
	exists, existsErr := a.BranchExists(repoPath, branch)
	if existsErr != nil {
		return fmt.Errorf("%w (also failed to check branch: %v)", err, existsErr)
	}
	if exists {
		// Branch exists; original failure is something else (path busy, etc.).
		return err
	}

	_, err2 := r.Git(repoPath, "worktree", "add", "-b", branch, worktreePath, from)
	if err2 != nil {
		return err2
	}
	return nil
}

// Remove removes a registered worktree at worktreePath from repoPath.
// When force is true, passes --force (dirty / locked worktrees).
func (a *Adapter) Remove(repoPath, worktreePath string, force bool) error {
	if repoPath == "" {
		return fmt.Errorf("git remove: repo path is empty")
	}
	if worktreePath == "" {
		return fmt.Errorf("git remove: worktree path is empty (repo %s)", repoPath)
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)

	_, err := a.runner().Git(repoPath, args...)
	return err
}

// Worktree is one entry from git worktree list --porcelain.
type Worktree struct {
	Path     string
	HEAD     string
	Branch   string // short name (no refs/heads/); empty if detached / bare
	Bare     bool
	Detached bool
}

// List returns worktrees registered for the repo at repoPath.
func (a *Adapter) List(repoPath string) ([]Worktree, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("git list: repo path is empty")
	}
	out, err := a.runner().Git(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreePorcelain(out), nil
}

// BranchExists reports whether refs/heads/<branch> exists in repoPath.
func (a *Adapter) BranchExists(repoPath, branch string) (bool, error) {
	if repoPath == "" {
		return false, fmt.Errorf("git branch-exists: repo path is empty")
	}
	if branch == "" {
		return false, fmt.Errorf("git branch-exists: branch is empty (repo %s)", repoPath)
	}
	ref := "refs/heads/" + branch
	_, err := a.runner().Git(repoPath, "show-ref", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}
	// show-ref exits 1 when the ref is missing; treat that as not exists.
	if ce, ok := err.(*CommandError); ok {
		if exitCode(ce) == 1 {
			return false, nil
		}
	}
	return false, err
}

func exitCode(ce *CommandError) int {
	if ce == nil || ce.Err == nil {
		return -1
	}
	var ee *exec.ExitError
	if errors.As(ce.Err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func parseWorktreePorcelain(out string) []Worktree {
	var (
		entries []Worktree
		cur     Worktree
		have    bool
	)
	flush := func() {
		if !have {
			return
		}
		entries = append(entries, cur)
		cur = Worktree{}
		have = false
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur.Path = val
			have = true
		case "HEAD":
			cur.HEAD = val
			have = true
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
			have = true
		case "detached":
			cur.Detached = true
			have = true
		case "bare":
			cur.Bare = true
			have = true
		}
	}
	flush()
	return entries
}
