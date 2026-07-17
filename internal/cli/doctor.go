package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/doctor"
	"github.com/spf13/cobra"
)

type doctorOptions struct {
	fix       bool
	continueOn bool
}

func newDoctorCmd(d Deps) *cobra.Command {
	var opts doctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect worktree registrations vs disk and suggest fixes",
		Long: `Compare git worktree list with the filesystem and report issues (§5.3).

Checks:
  - registered worktree paths that are missing (prunable) → prune + mwt add
  - disk directories under the worktree_path template that are not registered
  - missing meta root / main checkouts
  - missing setup copy destinations (from configured copy steps) → mwt setup

With --fix, automatically re-runs setup for all setup_missing findings
(grouped by branch across repos). Does not prune, delete unregistered
directories, or run mwt add.

Without --fix, only prints a report and suggested commands.
Suggested add/setup paths follow the resolved worktree_path template (dual defaults).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(d, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.fix, "fix", false, "re-run setup for all setup_missing findings")
	cmd.Flags().BoolVar(&opts.continueOn, "continue", false, "best-effort: continue after a setup failure during --fix; still exit non-zero if any failed")
	return cmd
}

func runDoctor(d Deps, opts doctorOptions) error {
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}

	checker := &doctor.Checker{
		Git: d.git(),
		FS:  doctor.OSFS{},
	}
	findings, err := checker.Check(cfg)
	if err != nil {
		return err
	}

	out := d.stdout()
	if err := doctor.FormatReport(out, findings); err != nil {
		return err
	}

	if !opts.fix {
		if len(findings) > 0 {
			return fmt.Errorf("doctor found %d issue(s)", len(findings))
		}
		return nil
	}

	if !hasSetupMissing(findings) {
		if len(findings) > 0 {
			return fmt.Errorf("doctor found %d issue(s)", len(findings))
		}
		return nil
	}

	fixErr := fixSetupMissing(d, cfg, findings, opts.continueOn)

	remaining, err := checker.Check(cfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(out)
	if fixErr != nil {
		fmt.Fprintln(out, "remaining after --fix:")
		if err := doctor.FormatReport(out, remaining); err != nil {
			return err
		}
		return fmt.Errorf("doctor --fix: %w", fixErr)
	}
	if len(remaining) == 0 {
		fmt.Fprintln(out, "fix: cleared setup_missing")
		fmt.Fprintln(out, "ok: no issues found")
		return nil
	}
	fmt.Fprintln(out, "remaining after --fix:")
	if err := doctor.FormatReport(out, remaining); err != nil {
		return err
	}
	return fmt.Errorf("doctor found %d issue(s)", len(remaining))
}

func hasSetupMissing(findings []doctor.Finding) bool {
	for _, f := range findings {
		if f.Kind == doctor.KindSetupMissing {
			return true
		}
	}
	return false
}

// fixSetupMissing runs setup for every setup_missing finding, grouped by branch.
// Repos within a branch follow cfg.Repos order.
func fixSetupMissing(d Deps, cfg *config.Config, findings []doctor.Finding, continueOn bool) error {
	byBranch := groupSetupMissingByBranch(cfg, findings)
	if len(byBranch) == 0 {
		return nil
	}

	out := d.stdout()
	branches := make([]string, 0, len(byBranch))
	for b := range byBranch {
		branches = append(branches, b)
	}
	sort.Strings(branches)

	var errs []error
	for _, branch := range branches {
		repos := byBranch[branch]
		fmt.Fprintf(out, "\nfix: setup %s --repos %s\n", branch, strings.Join(repos, ","))
		err := runSerial(repos, continueOn, func(repo string) error {
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
			fmt.Fprintf(out, "%s\t%s\n", repo, ctx.WorktreePath)
			return nil
		})
		if err != nil {
			if !continueOn {
				return err
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func groupSetupMissingByBranch(cfg *config.Config, findings []doctor.Finding) map[string][]string {
	repoOrder := make(map[string]int, len(cfg.Repos))
	for i, r := range cfg.Repos {
		repoOrder[r] = i
	}

	seen := make(map[string]map[string]struct{}) // branch → repos
	for _, f := range findings {
		if f.Kind != doctor.KindSetupMissing || f.Branch == "" || f.Repo == "" {
			continue
		}
		if seen[f.Branch] == nil {
			seen[f.Branch] = make(map[string]struct{})
		}
		seen[f.Branch][f.Repo] = struct{}{}
	}

	out := make(map[string][]string, len(seen))
	for branch, repos := range seen {
		list := make([]string, 0, len(repos))
		for r := range repos {
			list = append(list, r)
		}
		sort.SliceStable(list, func(i, j int) bool {
			oi, oki := repoOrder[list[i]]
			oj, okj := repoOrder[list[j]]
			if oki && okj {
				return oi < oj
			}
			if oki != okj {
				return oki
			}
			return list[i] < list[j]
		})
		out[branch] = list
	}
	return out
}
