package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type listOptions struct {
	repos      []string
	branch     string
	continueOn bool
}

func newListCmd(d Deps) *cobra.Command {
	var opts listOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List worktrees across configured repos",
		Long: `Aggregate git worktree list from each selected repo (serial).

Output columns: repo, branch, path (tab-separated).
Use --branch to keep only worktrees on that branch name.

Without --continue, the first repo list failure stops the command.
With --continue, remaining repos still run; the command exits non-zero
if any repo failed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(d, opts)
		},
	}
	cmd.Flags().StringSliceVar(&opts.repos, "repos", nil, "subset of config repos (comma-separated or repeated)")
	cmd.Flags().StringVar(&opts.branch, "branch", "", "filter by branch short name")
	cmd.Flags().BoolVar(&opts.continueOn, "continue", false, "best-effort: continue after a repo failure; still exit non-zero if any failed")
	return cmd
}

func runList(d Deps, opts listOptions) error {
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}
	repos, err := selectRepos(cfg, opts.repos)
	if err != nil {
		return err
	}

	out := d.stdout()
	return runSerial(repos, opts.continueOn, func(repo string) error {
		main := mainPath(cfg, repo)
		wts, err := d.git().List(main)
		if err != nil {
			return err
		}
		for _, wt := range wts {
			if wt.Bare {
				continue
			}
			branch := wt.Branch
			if branch == "" && wt.Detached {
				branch = "(detached)"
			}
			if opts.branch != "" && !branchMatches(branch, opts.branch) {
				continue
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", repo, branch, wt.Path)
		}
		return nil
	})
}

func branchMatches(actual, want string) bool {
	if actual == want {
		return true
	}
	// Accept accidental refs/heads/ prefix from callers.
	return strings.TrimPrefix(actual, "refs/heads/") == strings.TrimPrefix(want, "refs/heads/")
}
