package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type setupOptions struct {
	repos      []string
	continueOn bool
}

func newSetupCmd(d Deps) *cobra.Command {
	var opts setupOptions
	cmd := &cobra.Command{
		Use:   "setup <branch>",
		Short: "Run configured setup steps on existing worktrees",
		Long: `Re-run setup steps for <branch> on each selected repo (serial).

Requires the worktree directory to already exist (e.g. after
mwt add --no-setup, or a previous add). Resolves the same path template
as add/rm, then calls the shared setup runner (identical steps, cwd, and
failure semantics).

With --continue, remaining repos still run after a failure; the command
exits non-zero if any repo failed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(d, args[0], opts)
		},
	}
	cmd.Flags().StringSliceVar(&opts.repos, "repos", nil, "subset of config repos (comma-separated or repeated)")
	cmd.Flags().BoolVar(&opts.continueOn, "continue", false, "best-effort: continue after a repo failure; still exit non-zero if any failed")
	return cmd
}

func runSetup(d Deps, branch string, opts setupOptions) error {
	if branch == "" {
		return fmt.Errorf("branch is required")
	}
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}
	repos, err := selectRepos(cfg, opts.repos)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repos selected")
	}

	out := d.stdout()
	repoWidth := maxRepoWidth(repos)
	return runSerial(repos, opts.continueOn, func(repo string) error {
		ctx, err := resolveRepo(cfg, repo, branch)
		if err != nil {
			return err
		}
		if err := requireWorktreeDir(ctx.WorktreePath); err != nil {
			return err
		}
		if err := runSetupForRepo(d.setup(), cfg, ctx); err != nil {
			return err
		}
		fmt.Fprintf(out, "%-*s  %s\n", repoWidth, repo, displayPath(cfg.MetaRoot, ctx.WorktreePath))
		return nil
	})
}

// requireWorktreeDir ensures the resolved worktree path exists as a directory.
// setup is for re-running steps on existing worktrees only (not creating them).
func requireWorktreeDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree does not exist: %s (create it with mwt add first)", path)
		}
		return fmt.Errorf("stat worktree %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path is not a directory: %s", path)
	}
	return nil
}
