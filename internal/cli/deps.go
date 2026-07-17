package cli

import (
	"io"
	"os"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/git"
	"github.com/Slothtron/mwt/internal/pathresolve"
	"github.com/Slothtron/mwt/internal/setup"
)

// GitClient is the subset of git.Adapter used by core commands.
type GitClient interface {
	Add(repoPath, worktreePath, branch, from string) error
	Remove(repoPath, worktreePath string, force bool) error
	List(repoPath string) ([]git.Worktree, error)
}

// SetupRunner runs ordered setup steps for one worktree.
// Shared by `add` (default) and `setup` via runSetupForRepo.
type SetupRunner interface {
	Run(ctx pathresolve.Context, steps []config.SetupStep) error
}

// Deps holds injectable collaborators for CLI commands.
type Deps struct {
	Git        GitClient
	Setup      SetupRunner
	LoadConfig func() (*config.Config, error)
	MkdirAll   func(path string, perm os.FileMode) error
	Stdout     io.Writer
	Stderr     io.Writer
}

func defaultDeps() Deps {
	return Deps{
		Git:   git.New(),
		Setup: setup.New(),
		LoadConfig: func() (*config.Config, error) {
			wd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			return config.LoadFromDir(wd)
		},
		MkdirAll: os.MkdirAll,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}

func (d *Deps) stdout() io.Writer {
	if d != nil && d.Stdout != nil {
		return d.Stdout
	}
	return os.Stdout
}

func (d *Deps) stderr() io.Writer {
	if d != nil && d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

func (d *Deps) mkdirAll(path string, perm os.FileMode) error {
	if d != nil && d.MkdirAll != nil {
		return d.MkdirAll(path, perm)
	}
	return os.MkdirAll(path, perm)
}

func (d *Deps) loadConfig() (*config.Config, error) {
	if d != nil && d.LoadConfig != nil {
		return d.LoadConfig()
	}
	return defaultDeps().LoadConfig()
}

func (d *Deps) git() GitClient {
	if d != nil && d.Git != nil {
		return d.Git
	}
	return git.New()
}

func (d *Deps) setup() SetupRunner {
	if d != nil && d.Setup != nil {
		return d.Setup
	}
	return setup.New()
}
