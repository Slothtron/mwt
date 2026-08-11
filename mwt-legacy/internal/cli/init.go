package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/initscan"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const defaultInitDepth = 10

type initOptions struct {
	depth  int
	force  bool
	dryRun bool
}

// initFile is the on-disk shape for mwt init.
// Field order: root → worktree_path → repos → setup.
type initFile struct {
	Root         string   `yaml:"root"`
	WorktreePath string   `yaml:"worktree_path"`
	Repos        []string `yaml:"repos"`
	Setup        []any    `yaml:"setup"`
}

func newInitCmd() *cobra.Command {
	var opts initOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scan for git checkouts and write .mwt.yaml in the current directory",
		Long: `Discover Git repositories under the current directory (default max depth 10)
and write .mwt.yaml at the meta-root (cwd).

A directory is treated as a repo when it contains .git (directory or gitfile).
Once a repo is found, its subdirectories are not scanned. Directories named
.git, worktrees, and .worktrees are skipped.

worktree_path is filled from §5.1 dual-default:
  - cwd has .git  → .worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}
  - otherwise     → worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}
setup is an empty list.

Unlike other commands, init does not walk up looking for an existing .mwt.yaml.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, opts)
		},
	}
	cmd.Flags().IntVar(&opts.depth, "depth", defaultInitDepth, "max directory depth to scan (0 = current directory only)")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite existing .mwt.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the config that would be written without creating the file")
	return cmd
}

func runInit(cmd *cobra.Command, opts initOptions) error {
	if opts.depth < 0 {
		return fmt.Errorf("depth must be >= 0")
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	repos, err := initscan.Discover(wd, opts.depth)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no git repositories found under %s (depth=%d)", wd, opts.depth)
	}

	worktreePath := config.DefaultWorktreePathWithoutGit
	if config.GitExistsAt(wd) {
		worktreePath = config.DefaultWorktreePathWithGit
	}

	doc := initFile{
		Root:         ".",
		WorktreePath: worktreePath,
		Repos:        repos,
		Setup:        []any{},
	}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("encode .mwt.yaml: %w", err)
	}

	outPath := filepath.Join(wd, config.ConfigFileName)
	if opts.dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "# would write %s\n", outPath)
		_, err := cmd.OutOrStdout().Write(data)
		return err
	}

	if !opts.force {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", outPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", outPath, err)
		}
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d repos)\n", outPath, len(repos))
	fmt.Fprintf(cmd.OutOrStdout(), "worktree_path: %s\n", worktreePath)
	for _, r := range repos {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", r)
	}
	return nil
}
