// Package setup executes ordered setup steps (copy / run) for one worktree (§5.2, §6.2).
//
// It does not call git; callers (e.g. T06 add/setup commands) own worktree creation.
package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

// Runner executes config.SetupStep lists against a resolved pathresolve.Context.
type Runner struct {
	// Stdout and Stderr receive output from run steps. Defaults: os.Stdout / os.Stderr.
	// Copy never streams file contents to these writers.
	Stdout io.Writer
	Stderr io.Writer

	// Command, if set, runs shell commands (argv[0]=sh, argv[1]=-c, argv[2]=command).
	// Defaults to exec.Command. Tests may inject a stub.
	Command func(name string, arg ...string) *exec.Cmd
}

// New returns a Runner that uses the system shell and process stdio.
func New() *Runner {
	return &Runner{}
}

// Run executes steps in order for one repo worktree.
// An empty or nil steps slice is a no-op.
// Any step failure fails the whole setup (caller should treat as non-zero exit).
func (r *Runner) Run(ctx pathresolve.Context, steps []config.SetupStep) error {
	if ctx.Root == "" {
		return fmt.Errorf("setup: ROOT is empty")
	}
	if ctx.WorktreePath == "" {
		return fmt.Errorf("setup: WORKTREE_PATH is empty")
	}

	for i, step := range steps {
		if err := r.runStep(ctx, step); err != nil {
			return fmt.Errorf("setup: step %d: %w", i, err)
		}
	}
	return nil
}

func (r *Runner) runStep(ctx pathresolve.Context, step config.SetupStep) error {
	switch {
	case step.Copy != nil && step.Run != nil:
		return fmt.Errorf("step must have exactly one action")
	case step.Copy != nil:
		return r.copy(ctx, step.Copy)
	case step.Run != nil:
		return r.run(ctx, step.Run)
	default:
		return fmt.Errorf("step must have exactly one action")
	}
}

func (r *Runner) copy(ctx pathresolve.Context, action *config.CopyAction) error {
	fromRaw, err := ctx.Expand(action.From, pathresolve.StageSetup)
	if err != nil {
		return fmt.Errorf("copy.from: %w", err)
	}
	toRaw, err := ctx.Expand(action.To, pathresolve.StageSetup)
	if err != nil {
		return fmt.Errorf("copy.to: %w", err)
	}

	from := absOrJoin(fromRaw, ctx.Root)
	to := absOrJoin(toRaw, ctx.WorktreePath)

	if action.SkipIfExistsOrDefault() {
		if _, err := os.Lstat(to); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("copy: stat destination %s: %w", to, err)
		}
	}

	srcInfo, err := os.Stat(from)
	if err != nil {
		if os.IsNotExist(err) && action.SkipIfMissingSrcOrDefault() {
			return nil
		}
		return fmt.Errorf("copy: source %s: %w", from, err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("copy: source %s is a directory", from)
	}

	if err := copyFile(from, to, srcInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", from, to, err)
	}
	return nil
}

func (r *Runner) run(ctx pathresolve.Context, action *config.RunAction) error {
	command, err := ctx.Expand(action.Command, pathresolve.StageSetup)
	if err != nil {
		return fmt.Errorf("run.command: %w", err)
	}
	if command == "" {
		return fmt.Errorf("run.command is empty after expansion")
	}

	dir := ctx.WorktreePath
	if action.Dir != "" {
		dirRaw, err := ctx.Expand(action.Dir, pathresolve.StageSetup)
		if err != nil {
			return fmt.Errorf("run.dir: %w", err)
		}
		dir = absOrJoin(dirRaw, ctx.WorktreePath)
	}

	cmd := r.command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	// Intentionally do not log or dump env / secret file contents.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %q (cwd %s): %w", command, dir, err)
	}
	return nil
}

func (r *Runner) command(name string, arg ...string) *exec.Cmd {
	if r != nil && r.Command != nil {
		return r.Command(name, arg...)
	}
	return exec.Command(name, arg...)
}

func (r *Runner) stdout() io.Writer {
	if r != nil && r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r *Runner) stderr() io.Writer {
	if r != nil && r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

// absOrJoin returns cleaned absolute path: absolute inputs stay; relative join to base.
func absOrJoin(path, base string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

func copyFile(from, to string, perm os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
