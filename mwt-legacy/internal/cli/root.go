package cli

import (
	"fmt"
	"os"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/version"
	"github.com/spf13/cobra"
)

// NewRootCmd builds the mwt root command with default dependencies.
func NewRootCmd() *cobra.Command {
	return NewRootCmdWith(defaultDeps())
}

// NewRootCmdWith builds the mwt root command using the given deps (tests).
func NewRootCmdWith(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mwt",
		Short: "Multi-repo WorkTrees: polyrepo git worktree manager",
		Long: `mwt (Multi-repo WorkTrees) manages git worktrees across multiple
independent repositories from a single .mwt.yaml at the meta-root.`,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetVersionTemplate("{{.Version}}\n")

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newSkillCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newAddCmd(d))

	cmd.AddCommand(newRmCmd(d))
	cmd.AddCommand(newListCmd(d))
	cmd.AddCommand(newPathCmd(d))
	cmd.AddCommand(newSetupCmd(d))
	cmd.AddCommand(newDoctorCmd(d))
	cmd.AddCommand(newRootInfoCmd())
	return cmd
}

// Execute runs the root command.
func Execute() error {
	cmd := NewRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// newRootInfoCmd is a Phase-1 helper to verify meta-root discovery.
func newRootInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "meta-root",
		Short:  "Print the meta-root located by walking up for .mwt.yaml",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfgPath, err := config.FindConfigFile(wd)
			if err != nil {
				return err
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), cfg.MetaRoot)
			return nil
		},
	}
}
