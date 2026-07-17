package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type rmOptions struct {
	repos      []string
	force      bool
	continueOn bool
}

func newRmCmd(d Deps) *cobra.Command {
	var opts rmOptions
	cmd := &cobra.Command{
		Use:   "rm <branch>",
		Short: "Remove worktrees for the branch across configured repos",
		Long: `Remove the git worktree for <branch> in each selected repo (serial).

Uses git worktree remove. Dirty / locked / residual worktrees require --force.

With --continue, remaining repos still run after a failure; the command
exits non-zero if any repo failed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRm(d, args[0], opts)
		},
	}
	cmd.Flags().StringSliceVar(&opts.repos, "repos", nil, "subset of config repos (comma-separated or repeated)")
	cmd.Flags().BoolVar(&opts.force, "force", false, "pass --force to git worktree remove (dirty/locked/residual)")
	cmd.Flags().BoolVar(&opts.continueOn, "continue", false, "best-effort: continue after a repo failure; still exit non-zero if any failed")
	return cmd
}

func runRm(d Deps, branch string, opts rmOptions) error {
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
	return runSerial(repos, opts.continueOn, func(repo string) error {
		ctx, err := resolveRepo(cfg, repo, branch)
		if err != nil {
			return err
		}
		if err := d.git().Remove(ctx.MainPath, ctx.WorktreePath, opts.force); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\t%s\n", repo, ctx.WorktreePath)
		return nil
	})
}
