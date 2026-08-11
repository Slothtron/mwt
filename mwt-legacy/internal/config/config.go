package config

import "fmt"

// Default worktree path templates (§5.1).
const (
	DefaultWorktreePathWithGit    = ".worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
	DefaultWorktreePathWithoutGit = "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"
)

// Config is the parsed and validated .mwt.yaml.
type Config struct {
	// Root is relative to the config file directory; default ".".
	Root string `yaml:"root"`
	// WorktreePath is the path template. Omitted configs get a §5.1 default.
	WorktreePath string `yaml:"worktree_path"`
	// Repos are main-checkout paths relative to MetaRoot; also {{REPO}} values.
	Repos []string `yaml:"repos"`
	// Setup is an ordered list of setup steps (copy / run).
	Setup []SetupStep `yaml:"setup"`

	// MetaRoot is the absolute resolved meta-root (config dir + Root).
	MetaRoot string `yaml:"-"`
	// ConfigPath is the absolute path to .mwt.yaml.
	ConfigPath string `yaml:"-"`
	// HasGitAtRoot is true when {MetaRoot}/.git exists (dir or gitfile).
	HasGitAtRoot bool `yaml:"-"`
}

// SetupStep is a single-key action object (copy | run).
type SetupStep struct {
	Copy *CopyAction
	Run  *RunAction
}

// CopyAction copies a file into the worktree.
type CopyAction struct {
	From             string `yaml:"from"`
	To               string `yaml:"to"`
	SkipIfExists     *bool  `yaml:"skip_if_exists"`
	SkipIfMissingSrc *bool  `yaml:"skip_if_missing_src"`
}

// RunAction runs a shell command.
type RunAction struct {
	Command string `yaml:"command"`
	Dir     string `yaml:"dir"`
}

// SkipIfExistsOrDefault returns skip_if_exists, defaulting to true.
func (c *CopyAction) SkipIfExistsOrDefault() bool {
	if c.SkipIfExists == nil {
		return true
	}
	return *c.SkipIfExists
}

// SkipIfMissingSrcOrDefault returns skip_if_missing_src, defaulting to true.
func (c *CopyAction) SkipIfMissingSrcOrDefault() bool {
	if c.SkipIfMissingSrc == nil {
		return true
	}
	return *c.SkipIfMissingSrc
}

// Validate checks required fields after unmarshal / defaults.
func (c *Config) Validate() error {
	if c.MetaRoot == "" {
		return fmt.Errorf("config: meta root is empty")
	}
	if c.WorktreePath == "" {
		return fmt.Errorf("config: worktree_path is empty")
	}
	for i, repo := range c.Repos {
		if repo == "" {
			return fmt.Errorf("config: repos[%d] is empty", i)
		}
	}
	for i, step := range c.Setup {
		if err := step.validate(); err != nil {
			return fmt.Errorf("config: setup[%d]: %w", i, err)
		}
	}
	return nil
}

func (s SetupStep) validate() error {
	switch {
	case s.Copy != nil && s.Run != nil:
		return fmt.Errorf("step must have exactly one action")
	case s.Copy != nil:
		if s.Copy.From == "" {
			return fmt.Errorf("copy.from is required")
		}
		if s.Copy.To == "" {
			return fmt.Errorf("copy.to is required")
		}
		return nil
	case s.Run != nil:
		if s.Run.Command == "" {
			return fmt.Errorf("run.command is required")
		}
		return nil
	default:
		return fmt.Errorf("step must have exactly one action")
	}
}
