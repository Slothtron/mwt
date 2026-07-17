package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPathCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "path <branch> <repo>",
		Short: "Print the absolute worktree path for branch and repo",
		Long: `Resolve and print only the absolute worktree path for <repo> on <branch>.

Uses the same worktree_path template as add/rm (including §5.1 dual defaults).
Does not require the worktree to exist on disk.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPath(d, args[0], args[1])
		},
	}
}

func runPath(d Deps, branch, repo string) error {
	if branch == "" {
		return fmt.Errorf("branch is required")
	}
	if repo == "" {
		return fmt.Errorf("repo is required")
	}
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}
	// Validate repo is in config (path is still deterministic from template).
	if _, err := selectRepos(cfg, []string{repo}); err != nil {
		return err
	}
	ctx, err := resolveRepo(cfg, repo, branch)
	if err != nil {
		return err
	}
	fmt.Fprintln(d.stdout(), ctx.WorktreePath)
	return nil
}
