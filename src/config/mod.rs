//! `.mwt.yaml` 配置模型 + 加载/查找。
//!
//! 行为契约:§5.1 双默认规则、§5.2 `SetupStep` 单键契约。

use std::path::PathBuf;

use serde::Deserialize;
use thiserror::Error;

pub mod find;
pub mod load;
mod setup_step;

pub use find::{CONFIG_FILE_NAME, find_config_file};
pub use load::{git_exists_at, load, load_from_dir};
pub use setup_step::SetupStepWire;

/// Default worktree path template when the meta-root itself is a Git repo.
pub const DEFAULT_WORKTREE_PATH_WITH_GIT: &str = ".worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}";

/// Default worktree path template when the meta-root is NOT a Git repo
/// (typical polyrepo layout).
pub const DEFAULT_WORKTREE_PATH_WITHOUT_GIT: &str = "worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}";

/// Parsed and validated `.mwt.yaml`.
///
/// `meta_root` / `config_path` / `has_git_at_root` are populated by [`load`]
/// after deserialization; they are skipped in serde.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Config {
    /// Relative to the config file directory; default `"."`.
    pub root: String,
    /// Path template for the worktree location. Empty means "apply §5.1 default".
    pub worktree_path: String,
    /// Main-checkout paths relative to `meta_root`; also `{{REPO}}` values.
    pub repos: Vec<String>,
    /// Ordered list of setup steps.
    pub setup: Vec<SetupStep>,

    /// Absolute meta-root path (config dir + `root`).
    pub meta_root: PathBuf,
    /// Absolute path to the loaded `.mwt.yaml`.
    pub config_path: PathBuf,
    /// Whether `{meta_root}/.git` exists (directory or gitfile).
    pub has_git_at_root: bool,
}

/// A single setup step. Exactly one action must be set.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SetupStep {
    pub copy: Option<CopyAction>,
    pub run: Option<RunAction>,
}

impl SetupStep {
    pub fn copy(action: CopyAction) -> Self {
        Self {
            copy: Some(action),
            run: None,
        }
    }
    pub fn run(action: RunAction) -> Self {
        Self {
            copy: None,
            run: Some(action),
        }
    }
}

/// `copy:` step — copy a file from `from` to `to` inside the worktree.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct CopyAction {
    pub from: String,
    pub to: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skip_if_exists: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skip_if_missing_src: Option<bool>,
}

impl CopyAction {
    /// `skip_if_exists` defaulting to `true`.
    pub fn skip_if_exists_or_default(&self) -> bool {
        self.skip_if_exists.unwrap_or(true)
    }
    /// `skip_if_missing_src` defaulting to `true`.
    pub fn skip_if_missing_src_or_default(&self) -> bool {
        self.skip_if_missing_src.unwrap_or(true)
    }
}

/// `run:` step — run a shell command inside the worktree.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub struct RunAction {
    pub command: String,
    #[serde(default)]
    pub dir: Option<String>,
}

impl Config {
    /// Field-level validation mirroring Go's `(*Config).Validate`.
    pub fn validate(&self) -> Result<(), ConfigError> {
        if self.meta_root.as_os_str().is_empty() {
            return Err(ConfigError::Empty("meta root".into()));
        }
        if self.worktree_path.is_empty() {
            return Err(ConfigError::Empty("worktree_path".into()));
        }
        for (i, repo) in self.repos.iter().enumerate() {
            if repo.is_empty() {
                return Err(ConfigError::EmptyRepo(i));
            }
        }
        for (i, step) in self.setup.iter().enumerate() {
            step.validate().map_err(|e| ConfigError::SetupStep {
                index: i,
                source: Box::new(e),
            })?;
        }
        Ok(())
    }
}

impl SetupStep {
    fn validate(&self) -> Result<(), SetupStepError> {
        match (&self.copy, &self.run) {
            (Some(_), Some(_)) => Err(SetupStepError::MultipleActions),
            (Some(c), None) => {
                if c.from.is_empty() {
                    return Err(SetupStepError::Copy("from".into()));
                }
                if c.to.is_empty() {
                    return Err(SetupStepError::Copy("to".into()));
                }
                Ok(())
            }
            (None, Some(r)) => {
                if r.command.is_empty() {
                    return Err(SetupStepError::Run("command".into()));
                }
                Ok(())
            }
            (None, None) => Err(SetupStepError::NoAction),
        }
    }
}

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("config: {0} is empty")]
    Empty(String),
    #[error("config: repos[{0}] is empty")]
    EmptyRepo(usize),
    #[error("config: setup[{index}]: {source}")]
    SetupStep {
        index: usize,
        #[source]
        source: Box<SetupStepError>,
    },
}

#[derive(Debug, Error)]
pub enum SetupStepError {
    #[error("step must have exactly one action")]
    MultipleActions,
    #[error("step must have exactly one action")]
    NoAction,
    #[error("copy.{0} is required")]
    Copy(String),
    #[error("run.{0} is required")]
    Run(String),
}
