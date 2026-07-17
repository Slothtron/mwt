package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Slothtron/mwt/internal/pathresolve"
)

var placeholderRE = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// discoverDiskWorktrees walks cfg.WorktreePath with {{BRANCH}} as a wildcard
// for one repo. Templates without {{BRANCH}} yield no disk discoveries.
func discoverDiskWorktrees(fsys FS, metaRoot, template, repo string) ([]diskWorktree, error) {
	if !strings.Contains(template, "{{BRANCH}}") {
		return nil, nil
	}

	mainPath := filepath.Join(metaRoot, repo)
	expanded, err := expandTemplateLiterals(template, map[string]string{
		pathresolve.PhRoot:     metaRoot,
		pathresolve.PhRepo:     repo,
		pathresolve.PhRepoPath: repo,
		pathresolve.PhMainPath: mainPath,
	})
	if err != nil {
		return nil, err
	}

	pattern := expanded
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(metaRoot, pattern)
	}
	pattern = filepath.Clean(pattern)

	var out []diskWorktree
	if err := walkBranchPattern(fsys, pattern, "", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// expandTemplateLiterals replaces known placeholders except {{BRANCH}}.
func expandTemplateLiterals(template string, values map[string]string) (string, error) {
	var firstErr error
	out := placeholderRE.ReplaceAllStringFunc(template, func(match string) string {
		if firstErr != nil {
			return match
		}
		name := match[2 : len(match)-2]
		if name == pathresolve.PhBranch {
			return match // keep as wildcard marker
		}
		if name == pathresolve.PhWorktreePath || name == pathresolve.PhWorktreeName {
			firstErr = fmt.Errorf("doctor: placeholder %s is forbidden in worktree_path", match)
			return match
		}
		val, ok := values[name]
		if !ok {
			firstErr = fmt.Errorf("doctor: unknown placeholder %s in worktree_path", match)
			return match
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

// walkBranchPattern walks an absolute path pattern containing "{{BRANCH}}" segments.
func walkBranchPattern(fsys FS, pattern, branchSoFar string, out *[]diskWorktree) error {
	const marker = "{{BRANCH}}"
	idx := strings.Index(pattern, marker)
	if idx < 0 {
		info, err := fsys.Stat(pattern)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("doctor: stat %s: %w", pattern, err)
		}
		if info.IsDir() && branchSoFar != "" {
			*out = append(*out, diskWorktree{Branch: branchSoFar, Path: filepath.Clean(pattern)})
		}
		return nil
	}

	prefix := pattern[:idx]
	suffix := pattern[idx+len(marker):]
	// prefix should end with a path separator when marker is a full segment.
	parent := strings.TrimSuffix(prefix, string(filepath.Separator))
	if parent == "" {
		parent = string(filepath.Separator)
	}
	parent = filepath.Clean(parent)

	entries, err := fsys.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("doctor: read dir %s: %w", parent, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		next := filepath.Clean(prefix + name + suffix)
		br := name
		if branchSoFar != "" {
			br = branchSoFar
		}
		if err := walkBranchPattern(fsys, next, br, out); err != nil {
			return err
		}
	}
	return nil
}
