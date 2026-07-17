package cli

import (
	"fmt"

	"github.com/Slothtron/mwt/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Inspect worktree registrations vs disk and suggest fixes",
		Long: `Compare git worktree list with the filesystem and report issues (§5.3).

Checks:
  - registered worktree paths that are missing (prunable) → prune + mwt add
  - disk directories under the worktree_path template that are not registered
  - missing meta root / main checkouts
  - missing setup copy destinations (from configured copy steps) → mwt setup

Only prints a report and suggested commands; never deletes paths automatically.
Suggested add/setup paths follow the resolved worktree_path template (dual defaults).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(d)
		},
	}
}

func runDoctor(d Deps) error {
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

	if err := doctor.FormatReport(d.stdout(), findings); err != nil {
		return err
	}
	if len(findings) > 0 {
		return fmt.Errorf("doctor found %d issue(s)", len(findings))
	}
	return nil
}
