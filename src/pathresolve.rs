//! 渲染 `{{NAME}}` 路径占位符(§6.1)。
//!
//! 与 Go 版 `mwt-legacy/internal/pathresolve/` 行为一致。

use std::path::{Path, PathBuf};

use once_cell::sync::Lazy;
use regex::Regex;
use thiserror::Error;

use crate::config::Config;

pub const PH_ROOT: &str = "ROOT";
pub const PH_REPO: &str = "REPO";
pub const PH_REPO_PATH: &str = "REPO_PATH";
pub const PH_MAIN_PATH: &str = "MAIN_PATH";
pub const PH_BRANCH: &str = "BRANCH";
pub const PH_WORKTREE_PATH: &str = "WORKTREE_PATH";
pub const PH_WORKTREE_NAME: &str = "WORKTREE_NAME";

/// Which placeholders are allowed during expansion.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Stage {
    /// Forbid WORKTREE_PATH and WORKTREE_NAME (self-reference).
    WorktreePath,
    /// Allow all known placeholders (worktree path is already resolved).
    Setup,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Context {
    pub root: PathBuf,
    pub repo: String,
    pub repo_path: String,
    pub main_path: PathBuf,
    pub branch: String,
    pub worktree_path: PathBuf,
    pub worktree_name: String,
}

impl Context {
    /// Build a Context from `meta_root`, `worktree_path` template, `repo`, and `branch`.
    pub fn resolve(
        meta_root: &Path,
        worktree_path_template: &str,
        repo: &str,
        branch: &str,
    ) -> Result<Self, ResolveError> {
        if meta_root.as_os_str().is_empty() {
            return Err(ResolveError::Empty("meta root"));
        }
        if !meta_root.is_absolute() {
            return Err(ResolveError::NotAbsolute("meta root", meta_root.to_path_buf()));
        }
        if worktree_path_template.is_empty() {
            return Err(ResolveError::Empty("worktree_path template"));
        }
        if repo.is_empty() {
            return Err(ResolveError::Empty("repo"));
        }
        if branch.is_empty() {
            return Err(ResolveError::Empty("branch"));
        }

        let root = clean(meta_root);
        let main_path = root.join(repo);

        let partial = Context {
            root: root.clone(),
            repo: repo.to_string(),
            repo_path: repo.to_string(),
            main_path: main_path.clone(),
            branch: branch.to_string(),
            worktree_path: PathBuf::new(),
            worktree_name: String::new(),
        };

        let rendered = partial.expand(worktree_path_template, Stage::WorktreePath)?;
        let worktree_path = if Path::new(&rendered).is_absolute() {
            clean(Path::new(&rendered))
        } else {
            clean(&root.join(&rendered))
        };
        let worktree_name = worktree_path
            .file_name()
            .map(|s| s.to_string_lossy().to_string())
            .unwrap_or_default();

        Ok(Context {
            worktree_path,
            worktree_name,
            ..partial
        })
    }

    /// Build a Context using fields from a loaded [`Config`].
    pub fn resolve_from_config(cfg: &Config, repo: &str, branch: &str) -> Result<Self, ResolveError> {
        Self::resolve(&cfg.meta_root, &cfg.worktree_path, repo, branch)
    }

    /// Expand `{{NAME}}` placeholders in `s` according to the stage rules.
    pub fn expand(&self, s: &str, stage: Stage) -> Result<String, ExpandError> {
        let values = self.values_for_stage(stage)?;

        let mut err: Option<ExpandError> = None;
        let out = PLACEHOLDER_RE.replace_all(s, |caps: &regex::Captures<'_>| {
            if err.is_some() {
                return caps[0].to_string();
            }
            let name = caps[1].trim();
            if name.is_empty() {
                err = Some(ExpandError::EmptyPlaceholder(caps[0].to_string()));
                return caps[0].to_string();
            }
            if name.len() != caps[1].len() {
                err = Some(ExpandError::UnknownPlaceholder(caps[0].to_string()));
                return caps[0].to_string();
            }
            match values.get(name) {
                Some(v) => v.clone(),
                None => {
                    if stage == Stage::WorktreePath
                        && (name == PH_WORKTREE_PATH || name == PH_WORKTREE_NAME)
                    {
                        err = Some(ExpandError::ForbiddenInWorktreePath(caps[0].to_string()));
                    } else {
                        err = Some(ExpandError::UnknownPlaceholder(caps[0].to_string()));
                    }
                    caps[0].to_string()
                }
            }
        });

        if let Some(e) = err {
            return Err(e);
        }
        Ok(out.into_owned())
    }

    fn values_for_stage(&self, stage: Stage) -> Result<std::collections::HashMap<&'static str, String>, ExpandError> {
        use std::collections::HashMap;
        let mut base: HashMap<&'static str, String> = HashMap::new();
        base.insert(PH_ROOT, self.root.to_string_lossy().to_string());
        base.insert(PH_REPO, self.repo.clone());
        base.insert(PH_REPO_PATH, self.repo_path.clone());
        base.insert(PH_MAIN_PATH, self.main_path.to_string_lossy().to_string());
        base.insert(PH_BRANCH, self.branch.clone());
        match stage {
            Stage::WorktreePath => Ok(base),
            Stage::Setup => {
                if self.worktree_path.as_os_str().is_empty() {
                    return Err(ExpandError::WorktreePathUnresolved);
                }
                base.insert(PH_WORKTREE_PATH, self.worktree_path.to_string_lossy().to_string());
                base.insert(PH_WORKTREE_NAME, self.worktree_name.clone());
                Ok(base)
            }
        }
    }
}

static PLACEHOLDER_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"\{\{([^{}]*)\}\}").unwrap());

fn clean(p: &Path) -> PathBuf {
    // Path::clean on stable is nightly-only; emulate with components.
    let mut out = PathBuf::new();
    for comp in p.components() {
        match comp {
            std::path::Component::ParentDir => {
                out.pop();
            }
            std::path::Component::CurDir => {}
            other => out.push(other.as_os_str()),
        }
    }
    if out.as_os_str().is_empty() {
        PathBuf::from(".")
    } else {
        out
    }
}

#[derive(Debug, Error)]
pub enum ResolveError {
    #[error("pathresolve: {0} is empty")]
    Empty(&'static str),
    #[error("pathresolve: {0} must be absolute: {1:?}")]
    NotAbsolute(&'static str, PathBuf),
    #[error("pathresolve: {0}")]
    Expand(#[from] ExpandError),
}

#[derive(Debug, Error)]
pub enum ExpandError {
    #[error("pathresolve: unknown placeholder {0}")]
    UnknownPlaceholder(String),
    #[error("pathresolve: placeholder {0} is forbidden in worktree_path")]
    ForbiddenInWorktreePath(String),
    #[error("pathresolve: empty placeholder {0}")]
    EmptyPlaceholder(String),
    #[error("pathresolve: WORKTREE_PATH is not resolved")]
    WorktreePathUnresolved,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ctx_with(branch: &str) -> Context {
        Context::resolve(
            Path::new("/meta"),
            ".worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}",
            "api",
            branch,
        )
        .unwrap()
    }

    #[test]
    fn renders_template_at_worktree_path_stage() {
        let c = ctx_with("feat-x");
        assert_eq!(c.worktree_path, PathBuf::from("/meta/.worktrees/api/feat-x/api"));
        assert_eq!(c.worktree_name, "api");
        assert_eq!(c.main_path, PathBuf::from("/meta/api"));
    }

    #[test]
    fn forbidden_in_worktree_path_stage() {
        let c = ctx_with("feat-x");
        let err = c.expand("{{WORKTREE_PATH}}", Stage::WorktreePath).unwrap_err();
        assert!(matches!(err, ExpandError::ForbiddenInWorktreePath(_)));
    }

    #[test]
    fn allowed_in_setup_stage() {
        let c = ctx_with("feat-x");
        let out = c.expand("cp {{ROOT}}/x {{WORKTREE_PATH}}/x", Stage::Setup).unwrap();
        assert_eq!(out, "cp /meta/x /meta/.worktrees/api/feat-x/api/x");
    }

    #[test]
    fn unknown_placeholder() {
        let c = ctx_with("feat-x");
        let err = c.expand("{{NOPE}}", Stage::WorktreePath).unwrap_err();
        assert!(matches!(err, ExpandError::UnknownPlaceholder(_)));
    }

    #[test]
    fn empty_placeholder() {
        let c = ctx_with("feat-x");
        let err = c.expand("{{}}", Stage::WorktreePath).unwrap_err();
        assert!(matches!(err, ExpandError::EmptyPlaceholder(_)));
    }

    #[test]
    fn setup_requires_resolved_worktree_path() {
        let c = Context {
            root: PathBuf::from("/meta"),
            repo: "api".into(),
            repo_path: "api".into(),
            main_path: PathBuf::from("/meta/api"),
            branch: "feat-x".into(),
            worktree_path: PathBuf::new(),
            worktree_name: String::new(),
        };
        let err = c.expand("{{WORKTREE_PATH}}", Stage::Setup).unwrap_err();
        assert!(matches!(err, ExpandError::WorktreePathUnresolved));
    }

    #[test]
    fn absolute_template_kept() {
        let c = Context::resolve(
            Path::new("/meta"),
            "/var/wt/{{REPO}}/{{BRANCH}}",
            "api",
            "main",
        )
        .unwrap();
        assert_eq!(c.worktree_path, PathBuf::from("/var/wt/api/main"));
    }

    #[test]
    fn rejects_relative_meta_root() {
        let err = Context::resolve(
            Path::new("rel"),
            "wt",
            "api",
            "main",
        )
        .unwrap_err();
        assert!(matches!(err, ResolveError::NotAbsolute(_, _)));
    }
}
