package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

type addOptions struct {
	repos      []string
	from       string
	noSetup    bool
	continueOn bool
}

func newAddCmd(d Deps) *cobra.Command {
	var opts addOptions
	cmd := &cobra.Command{
		Use:   "add <branch>",
		Short: "Create worktrees for the branch across configured repos",
		Long: `Create a git worktree for <branch> in each selected repo (serial).

For each repo: mkdir -p the worktree parent, git worktree add, then run
configured setup steps (unless --no-setup). Use --from to create the branch
from a start point when it does not exist yet.

With --continue, remaining repos still run after a failure; the command
exits non-zero if any repo failed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(d, args[0], opts)
		},
	}
	cmd.Flags().StringSliceVar(&opts.repos, "repos", nil, "subset of config repos (comma-separated or repeated)")
	cmd.Flags().StringVar(&opts.from, "from", "", "start point when creating a missing branch (git worktree add -b)")
	cmd.Flags().BoolVar(&opts.noSetup, "no-setup", false, "skip setup steps after creating the worktree")
	cmd.Flags().BoolVar(&opts.continueOn, "continue", false, "best-effort: continue after a repo failure; still exit non-zero if any failed")
	return cmd
}

func runAdd(d Deps, branch string, opts addOptions) error {
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
		parent := filepath.Dir(ctx.WorktreePath)
		if err := d.mkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", parent, err)
		}
		if err := d.git().Add(ctx.MainPath, ctx.WorktreePath, branch, opts.from); err != nil {
			return err
		}
		if !opts.noSetup {
			if err := runSetupForRepo(d.setup(), cfg, ctx); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "%-*s  %s\n", repoWidth, repo, displayPath(cfg.MetaRoot, ctx.WorktreePath))
		return nil
	})
}
