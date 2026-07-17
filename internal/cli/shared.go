package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

// selectRepos returns the configured repos to operate on.
// When requested is empty, all cfg.Repos are used.
// Unknown names are an error.
func selectRepos(cfg *config.Config, requested []string) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if len(requested) == 0 {
		return append([]string(nil), cfg.Repos...), nil
	}

	known := make(map[string]struct{}, len(cfg.Repos))
	for _, r := range cfg.Repos {
		known[r] = struct{}{}
	}

	seen := make(map[string]struct{}, len(requested))
	out := make([]string, 0, len(requested))
	for _, r := range requested {
		r = strings.TrimSpace(r)
		if r == "" {
			return nil, fmt.Errorf("empty repo name in --repos")
		}
		if _, ok := known[r]; !ok {
			return nil, fmt.Errorf("repo %q is not in config repos", r)
		}
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out, nil
}

// runSerial executes fn for each repo in order.
// Without continueOnErr, the first error stops the loop.
// With continueOnErr, remaining repos still run; any failure yields a joined
// non-nil error at the end (exit non-zero).
func runSerial(repos []string, continueOnErr bool, fn func(repo string) error) error {
	var errs []error
	for _, repo := range repos {
		if err := fn(repo); err != nil {
			wrapped := fmt.Errorf("%s: %w", repo, err)
			if !continueOnErr {
				return wrapped
			}
			errs = append(errs, wrapped)
		}
	}
	return errors.Join(errs...)
}

func resolveRepo(cfg *config.Config, repo, branch string) (pathresolve.Context, error) {
	return pathresolve.ResolveFromConfig(cfg, repo, branch)
}

func mainPath(cfg *config.Config, repo string) string {
	return filepath.Join(cfg.MetaRoot, repo)
}
