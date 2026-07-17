package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Slothtron/mwt/internal/skilldata"
	"github.com/spf13/cobra"
)

type skillSyncOptions struct {
	dir   string
	force bool
}

func newSkillCmd() *cobra.Command {
	var opts skillSyncOptions
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage the mwt Agent skill",
		Long: `Install or update the embedded mwt Agent skill into a skills directory.

Running mwt skill with no subcommand is equivalent to mwt skill sync.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillSync(cmd, opts)
		},
	}
	cmd.PersistentFlags().StringVar(&opts.dir, "dir", "", "parent skills directory (default: ~/.agents/skills)")
	cmd.PersistentFlags().BoolVar(&opts.force, "force", false, "overwrite existing mwt skill directory")

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync the embedded mwt skill to a skills directory",
		Long: `Copy the embedded Agent skill to <dir>/mwt (default: ~/.agents/skills/mwt).

Use --force to overwrite an existing installation.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillSync(cmd, opts)
		},
	}
	cmd.AddCommand(syncCmd)
	return cmd
}

func runSkillSync(cmd *cobra.Command, opts skillSyncOptions) error {
	parent := opts.dir
	if parent == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		parent = filepath.Join(home, ".agents", "skills")
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return err
	}

	dest, err := skilldata.Sync(parent, opts.force)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "synced skill to %s\n", dest)
	return nil
}
