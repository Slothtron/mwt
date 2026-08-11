// Package doctor compares disk state with git worktree registrations (§5.3).
//
// It only reports findings and suggested commands; it never deletes paths.
package doctor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/git"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

// Kind classifies a doctor finding.
type Kind string

const (
	// KindRootMissing: MetaRoot does not exist on disk.
	KindRootMissing Kind = "root_missing"
	// KindMainMissing: a configured repo main checkout is missing.
	KindMainMissing Kind = "main_missing"
	// KindPrunable: git worktree list path does not exist (prunable).
	KindPrunable Kind = "prunable"
	// KindUnregistered: disk directory looks like a worktree but is not registered.
	KindUnregistered Kind = "unregistered"
	// KindSetupMissing: a setup copy destination is missing in an existing worktree.
	KindSetupMissing Kind = "setup_missing"
)

// Finding is one reportable issue with optional repair suggestions.
type Finding struct {
	Kind    Kind
	Repo    string
	Branch  string
	Path    string
	Message string
	Suggest []string
}

// GitLister lists worktrees for a main-checkout path.
type GitLister interface {
	List(repoPath string) ([]git.Worktree, error)
}

// FS is the filesystem surface doctor needs (injectable for tests).
type FS interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

// OSFS uses the real OS filesystem.
type OSFS struct{}

func (OSFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
func (OSFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

// Checker runs §5.3 inspections.
type Checker struct {
	Git GitLister
	FS  FS
}

// Check inspects cfg for all configured repos.
// Path suggestions always go through pathresolve using cfg.WorktreePath
// (already dual-default resolved by config.Load).
func (c *Checker) Check(cfg *config.Config) ([]Finding, error) {
	if cfg == nil {
		return nil, fmt.Errorf("doctor: config is nil")
	}
	if c == nil || c.Git == nil {
		return nil, fmt.Errorf("doctor: git lister is nil")
	}
	fsys := c.fs()

	var findings []Finding

	if _, err := fsys.Stat(cfg.MetaRoot); err != nil {
		if os.IsNotExist(err) {
			findings = append(findings, Finding{
				Kind:    KindRootMissing,
				Path:    cfg.MetaRoot,
				Message: fmt.Sprintf("meta root missing: %s", cfg.MetaRoot),
				Suggest: []string{
					"# fix root in .mwt.yaml or create the meta-root directory",
				},
			})
			return findings, nil
		}
		return nil, fmt.Errorf("doctor: stat meta root %s: %w", cfg.MetaRoot, err)
	}

	for _, repo := range cfg.Repos {
		repoFindings, err := c.checkRepo(cfg, repo)
		if err != nil {
			return nil, err
		}
		findings = append(findings, repoFindings...)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].Repo != findings[j].Repo {
			return findings[i].Repo < findings[j].Repo
		}
		if findings[i].Branch != findings[j].Branch {
			return findings[i].Branch < findings[j].Branch
		}
		return findings[i].Path < findings[j].Path
	})
	return findings, nil
}

func (c *Checker) fs() FS {
	if c != nil && c.FS != nil {
		return c.FS
	}
	return OSFS{}
}

func (c *Checker) checkRepo(cfg *config.Config, repo string) ([]Finding, error) {
	fsys := c.fs()
	main := filepath.Join(cfg.MetaRoot, repo)

	if _, err := fsys.Stat(main); err != nil {
		if os.IsNotExist(err) {
			return []Finding{{
				Kind:    KindMainMissing,
				Repo:    repo,
				Path:    main,
				Message: fmt.Sprintf("main checkout missing: %s", main),
				Suggest: []string{
					"# fix .mwt.yaml repos entry or place the git checkout at the expected path",
				},
			}}, nil
		}
		return nil, fmt.Errorf("doctor: stat main %s: %w", main, err)
	}

	registered, err := c.Git.List(main)
	if err != nil {
		return nil, fmt.Errorf("doctor: list worktrees for %s: %w", repo, err)
	}

	regByPath := make(map[string]git.Worktree, len(registered))
	for _, wt := range registered {
		if wt.Bare || wt.Path == "" {
			continue
		}
		p := filepath.Clean(wt.Path)
		regByPath[p] = wt
	}

	var findings []Finding

	// Prunable: registered linked worktrees whose path is gone.
	for path, wt := range regByPath {
		if path == filepath.Clean(main) {
			continue // main checkout checked above
		}
		if _, err := fsys.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("doctor: stat worktree %s: %w", path, err)
		}

		branch := wt.Branch
		canonical := ""
		if branch != "" {
			if ctx, rerr := pathresolve.ResolveFromConfig(cfg, repo, branch); rerr == nil {
				canonical = ctx.WorktreePath
			}
		}
		suggest := []string{
			fmt.Sprintf("git -C %s worktree prune", main),
		}
		if branch != "" {
			suggest = append(suggest, fmt.Sprintf("mwt add %s --repos %s", branch, repo))
			if canonical != "" {
				suggest = append(suggest,
					fmt.Sprintf("# canonical path from worktree_path: %s", canonical))
			}
		} else {
			suggest = append(suggest, "# re-add via mwt add <branch> --repos "+repo+" after identifying the branch")
		}

		findings = append(findings, Finding{
			Kind:    KindPrunable,
			Repo:    repo,
			Branch:  branch,
			Path:    path,
			Message: fmt.Sprintf("registered worktree path missing (prunable): %s", path),
			Suggest: suggest,
		})
	}

	// Unregistered: disk dirs matching the template that git does not know.
	diskEntries, err := discoverDiskWorktrees(fsys, cfg.MetaRoot, cfg.WorktreePath, repo)
	if err != nil {
		return nil, err
	}
	for _, d := range diskEntries {
		if _, ok := regByPath[d.Path]; ok {
			continue
		}
		findings = append(findings, Finding{
			Kind:    KindUnregistered,
			Repo:    repo,
			Branch:  d.Branch,
			Path:    d.Path,
			Message: fmt.Sprintf("disk directory exists but is not a registered worktree: %s", d.Path),
			Suggest: []string{
				fmt.Sprintf("mwt add %s --repos %s", d.Branch, repo),
				"# or remove the unregistered directory manually (mwt will not auto-delete)",
			},
		})
	}

	// Setup copy destinations missing on existing, registered worktrees.
	if len(cfg.Setup) > 0 {
		for path, wt := range regByPath {
			if path == filepath.Clean(main) {
				continue
			}
			if _, err := fsys.Stat(path); err != nil {
				continue // prunable already reported
			}
			branch := wt.Branch
			if branch == "" {
				continue
			}
			ctx, err := pathresolve.ResolveFromConfig(cfg, repo, branch)
			if err != nil {
				return nil, err
			}
			// Prefer the actual registered path when it differs from the template.
			if filepath.Clean(ctx.WorktreePath) != path {
				ctx.WorktreePath = path
				ctx.WorktreeName = filepath.Base(path)
			}
			setupFindings, err := c.checkSetupCopies(fsys, cfg, ctx)
			if err != nil {
				return nil, err
			}
			findings = append(findings, setupFindings...)
		}
	}

	return findings, nil
}

func (c *Checker) checkSetupCopies(fsys FS, cfg *config.Config, ctx pathresolve.Context) ([]Finding, error) {
	var findings []Finding
	seenTo := make(map[string]struct{})

	for _, step := range cfg.Setup {
		if step.Copy == nil {
			continue
		}
		fromRaw, err := ctx.Expand(step.Copy.From, pathresolve.StageSetup)
		if err != nil {
			return nil, fmt.Errorf("doctor: expand copy.from: %w", err)
		}
		toRaw, err := ctx.Expand(step.Copy.To, pathresolve.StageSetup)
		if err != nil {
			return nil, fmt.Errorf("doctor: expand copy.to: %w", err)
		}
		from := absOrJoin(fromRaw, ctx.Root)
		to := absOrJoin(toRaw, ctx.WorktreePath)

		if _, ok := seenTo[to]; ok {
			continue
		}

		if _, err := fsys.Stat(to); err == nil {
			seenTo[to] = struct{}{}
			continue
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("doctor: stat setup dest %s: %w", to, err)
		}

		// Mirror setup skip_if_missing_src: if source is absent and skip is on, not a problem.
		if step.Copy.SkipIfMissingSrcOrDefault() {
			if _, err := fsys.Stat(from); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("doctor: stat setup src %s: %w", from, err)
			}
		}

		seenTo[to] = struct{}{}
		findings = append(findings, Finding{
			Kind:    KindSetupMissing,
			Repo:    ctx.Repo,
			Branch:  ctx.Branch,
			Path:    to,
			Message: fmt.Sprintf("setup copy destination missing: %s", to),
			Suggest: []string{
				fmt.Sprintf("mwt setup %s --repos %s", ctx.Branch, ctx.Repo),
			},
		})
	}
	return findings, nil
}

func absOrJoin(path, base string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

// diskWorktree is one on-disk candidate discovered from the path template.
type diskWorktree struct {
	Branch string
	Path   string
}
