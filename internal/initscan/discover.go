package initscan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// skipDirNames are never entered and never listed as repos.
var skipDirNames = map[string]struct{}{
	".git":       {},
	"worktrees":  {},
	".worktrees": {},
}

// Discover walks root (depth 0) downward up to maxDepth levels looking for
// Git checkouts (presence of .git as dir or gitfile). Relative paths are
// returned sorted. When a git root is found, its children are not scanned.
// maxDepth 0 means only root itself is checked.
func Discover(root string, maxDepth int) ([]string, error) {
	if maxDepth < 0 {
		return nil, fmt.Errorf("initscan: max depth must be >= 0")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("initscan: resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("initscan: root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("initscan: root is not a directory: %s", absRoot)
	}

	var repos []string
	if err := walk(absRoot, absRoot, 0, maxDepth, &repos); err != nil {
		return nil, err
	}
	sort.Strings(repos)
	return repos, nil
}

func walk(absRoot, dir string, depth, maxDepth int, repos *[]string) error {
	if hasGit(dir) {
		rel, err := relRepo(absRoot, dir)
		if err != nil {
			return err
		}
		*repos = append(*repos, rel)
		return nil // do not descend into a git checkout
	}

	if depth >= maxDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("initscan: read %s: %w", dir, err)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if _, skip := skipDirNames[name]; skip {
			continue
		}
		child := filepath.Join(dir, name)
		if err := walk(absRoot, child, depth+1, maxDepth, repos); err != nil {
			return err
		}
	}
	return nil
}

func hasGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func relRepo(absRoot, dir string) (string, error) {
	rel, err := filepath.Rel(absRoot, dir)
	if err != nil {
		return "", fmt.Errorf("initscan: relative path: %w", err)
	}
	if rel == "." {
		return ".", nil
	}
	// Normalize to slash-separated relative paths for YAML portability.
	return filepath.ToSlash(rel), nil
}
