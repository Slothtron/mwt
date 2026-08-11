package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and validates .mwt.yaml at configPath, applying §5.1 defaults.
func Load(configPath string) (*Config, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", absPath, err)
	}

	cfg.ConfigPath = absPath
	configDir := filepath.Dir(absPath)

	if cfg.Root == "" {
		cfg.Root = "."
	}
	metaRoot, err := filepath.Abs(filepath.Join(configDir, cfg.Root))
	if err != nil {
		return nil, fmt.Errorf("resolve meta root: %w", err)
	}
	cfg.MetaRoot = metaRoot
	cfg.HasGitAtRoot = GitExistsAt(metaRoot)

	// Dual default for worktree_path: only when omitted / empty.
	// Explicit values are kept as-is (no worktrees ↔ .worktrees rewrite).
	if cfg.WorktreePath == "" {
		if cfg.HasGitAtRoot {
			cfg.WorktreePath = DefaultWorktreePathWithGit
		} else {
			cfg.WorktreePath = DefaultWorktreePathWithoutGit
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadFromDir finds .mwt.yaml by walking up from startDir, then Load.
func LoadFromDir(startDir string) (*Config, error) {
	path, err := FindConfigFile(startDir)
	if err != nil {
		return nil, err
	}
	return Load(path)
}

// GitExistsAt reports whether {root}/.git exists as a directory or gitfile.
func GitExistsAt(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}
