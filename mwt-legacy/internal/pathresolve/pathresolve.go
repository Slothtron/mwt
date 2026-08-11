// Package pathresolve renders {{NAME}} path placeholders (§6.1).
package pathresolve

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Slothtron/mwt/internal/config"
)

// Stage controls which placeholders are allowed during expansion.
type Stage int

const (
	// StageWorktreePath forbids WORKTREE_PATH and WORKTREE_NAME (self-reference).
	StageWorktreePath Stage = iota
	// StageSetup allows all known placeholders after the worktree path is resolved.
	StageSetup
)

// Known placeholder names (§6.1).
const (
	PhRoot         = "ROOT"
	PhRepo         = "REPO"
	PhRepoPath     = "REPO_PATH"
	PhMainPath     = "MAIN_PATH"
	PhBranch       = "BRANCH"
	PhWorktreePath = "WORKTREE_PATH"
	PhWorktreeName = "WORKTREE_NAME"
)

var placeholderRE = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// Context holds resolved placeholder values for one repo + branch.
type Context struct {
	Root         string
	Repo         string
	RepoPath     string
	MainPath     string
	Branch       string
	WorktreePath string
	WorktreeName string
}

// Resolve builds a Context from meta root, worktree_path template, repo, and branch.
// Relative rendered worktree paths are joined to metaRoot and cleaned to an absolute path.
// The worktree_path template is expanded at StageWorktreePath (WORKTREE_* forbidden).
func Resolve(metaRoot, worktreePathTemplate, repo, branch string) (Context, error) {
	if metaRoot == "" {
		return Context{}, fmt.Errorf("pathresolve: meta root is empty")
	}
	if !filepath.IsAbs(metaRoot) {
		return Context{}, fmt.Errorf("pathresolve: meta root must be absolute: %q", metaRoot)
	}
	if worktreePathTemplate == "" {
		return Context{}, fmt.Errorf("pathresolve: worktree_path template is empty")
	}
	if repo == "" {
		return Context{}, fmt.Errorf("pathresolve: repo is empty")
	}
	if branch == "" {
		return Context{}, fmt.Errorf("pathresolve: branch is empty")
	}

	root := filepath.Clean(metaRoot)
	mainPath := filepath.Join(root, repo)

	partial := Context{
		Root:     root,
		Repo:     repo,
		RepoPath: repo,
		MainPath: mainPath,
		Branch:   branch,
	}

	rendered, err := partial.Expand(worktreePathTemplate, StageWorktreePath)
	if err != nil {
		return Context{}, err
	}

	worktreePath := rendered
	if !filepath.IsAbs(worktreePath) {
		worktreePath = filepath.Join(root, worktreePath)
	}
	worktreePath = filepath.Clean(worktreePath)

	partial.WorktreePath = worktreePath
	partial.WorktreeName = filepath.Base(worktreePath)
	return partial, nil
}

// ResolveFromConfig resolves a worktree path using fields from a loaded Config.
// It consumes MetaRoot and WorktreePath as already finalized by config loading
// (including §5.1 defaults); it does not re-decide .worktrees vs worktrees.
func ResolveFromConfig(cfg *config.Config, repo, branch string) (Context, error) {
	if cfg == nil {
		return Context{}, fmt.Errorf("pathresolve: config is nil")
	}
	return Resolve(cfg.MetaRoot, cfg.WorktreePath, repo, branch)
}

// Expand replaces {{NAME}} placeholders in s according to stage rules.
// Unknown placeholders and stage-forbidden names return an error.
func (c Context) Expand(s string, stage Stage) (string, error) {
	values, err := c.valuesForStage(stage)
	if err != nil {
		return "", err
	}

	var firstErr error
	out := placeholderRE.ReplaceAllStringFunc(s, func(match string) string {
		if firstErr != nil {
			return match
		}
		name := match[2 : len(match)-2] // strip {{ }}
		if strings.TrimSpace(name) != name || name == "" {
			firstErr = fmt.Errorf("pathresolve: unknown placeholder %s", match)
			return match
		}
		val, ok := values[name]
		if !ok {
			// Distinguish forbidden self-refs at worktree_path stage.
			if stage == StageWorktreePath && (name == PhWorktreePath || name == PhWorktreeName) {
				firstErr = fmt.Errorf("pathresolve: placeholder %s is forbidden in worktree_path", match)
				return match
			}
			firstErr = fmt.Errorf("pathresolve: unknown placeholder %s", match)
			return match
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func (c Context) valuesForStage(stage Stage) (map[string]string, error) {
	base := map[string]string{
		PhRoot:     c.Root,
		PhRepo:     c.Repo,
		PhRepoPath: c.RepoPath,
		PhMainPath: c.MainPath,
		PhBranch:   c.Branch,
	}
	switch stage {
	case StageWorktreePath:
		return base, nil
	case StageSetup:
		if c.WorktreePath == "" {
			return nil, fmt.Errorf("pathresolve: WORKTREE_PATH is not resolved")
		}
		base[PhWorktreePath] = c.WorktreePath
		base[PhWorktreeName] = c.WorktreeName
		return base, nil
	default:
		return nil, fmt.Errorf("pathresolve: unknown stage %d", stage)
	}
}
