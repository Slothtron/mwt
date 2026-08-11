//! Read `.mwt.yaml` from disk, apply defaults, and return a validated [`Config`].

use std::path::{Path, PathBuf};

use serde::Deserialize;
use thiserror::Error;

use super::{
    Config, ConfigError, DEFAULT_WORKTREE_PATH_WITH_GIT, DEFAULT_WORKTREE_PATH_WITHOUT_GIT,
    SetupStepWire,
};

/// Top-level wire format. Field names mirror the YAML keys exactly.
#[derive(Debug, Deserialize)]
struct ConfigWire {
    #[serde(default)]
    root: String,
    #[serde(default)]
    worktree_path: String,
    #[serde(default)]
    repos: Vec<String>,
    #[serde(default)]
    setup: Vec<SetupStepWire>,
}

/// Read and validate the config at `config_path`, applying §5.1 defaults.
pub fn load(config_path: &Path) -> Result<Config, LoadError> {
    let abs_path = config_path
        .canonicalize()
        .map_err(|e| LoadError::ResolvePath {
            path: config_path.to_path_buf(),
            source: e,
        })?;
    let data = std::fs::read(&abs_path).map_err(|e| LoadError::Read {
        path: abs_path.clone(),
        source: e,
    })?;
    let wire: ConfigWire = serde_saphyr::from_slice(&data).map_err(|e| LoadError::Parse {
        path: abs_path.clone(),
        message: format!("{e}"),
    })?;

    let config_dir = abs_path
        .parent()
        .ok_or_else(|| LoadError::InvalidPath(abs_path.clone()))?
        .to_path_buf();

    let root = if wire.root.is_empty() {
        ".".to_string()
    } else {
        wire.root
    };
    let meta_root = config_dir.join(&root).canonicalize().unwrap_or_else(|_| {
        // Mirrors Go: `filepath.Abs(filepath.Join(configDir, cfg.Root))`.
        // We use a non-canonicalizing path if the dir doesn't yet exist,
        // because `Load` is also valid for brand-new meta-roots.
        let joined = config_dir.join(&root);
        std::path::absolute(&joined).unwrap_or(joined)
    });

    let has_git_at_root = git_exists_at(&meta_root);

    // Dual default: only when omitted. Explicit values kept verbatim.
    let worktree_path = if wire.worktree_path.is_empty() {
        if has_git_at_root {
            DEFAULT_WORKTREE_PATH_WITH_GIT.to_string()
        } else {
            DEFAULT_WORKTREE_PATH_WITHOUT_GIT.to_string()
        }
    } else {
        wire.worktree_path
    };

    let cfg = Config {
        root,
        worktree_path,
        repos: wire.repos,
        setup: wire.setup.into_iter().map(|s| s.0).collect(),
        meta_root,
        config_path: abs_path.clone(),
        has_git_at_root,
    };

    cfg.validate().map_err(LoadError::Validate)?;
    Ok(cfg)
}

/// Find `.mwt.yaml` by walking up from `start_dir`, then load it.
pub fn load_from_dir(start_dir: &Path) -> Result<Config, LoadError> {
    let path = super::find_config_file(start_dir).map_err(|e| match e {
        super::find::FindError::NotFound(p) => LoadError::NotFound(p),
        super::find::FindError::Stat { path, source } => LoadError::Read { path, source },
    })?;
    load(&path)
}

/// `true` when `{root}/.git` exists as a directory or gitfile.
pub fn git_exists_at(root: &Path) -> bool {
    // `symlink_metadata` to follow gitfile indirection.
    std::fs::symlink_metadata(root.join(".git")).is_ok()
}

#[derive(Debug, Error)]
pub enum LoadError {
    #[error("resolve config path {path}: {source}")]
    ResolvePath {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("read {path}: {source}")]
    Read {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("parse {path}: {message}")]
    Parse { path: PathBuf, message: String },
    #[error("invalid config path: {0}")]
    InvalidPath(PathBuf),
    #[error("no {} found from {0} upward", super::CONFIG_FILE_NAME)]
    NotFound(PathBuf),
    #[error("{0}")]
    Validate(#[source] ConfigError),
}

#[cfg(test)]
mod tests {
    use super::*;

    fn write(dir: &Path, name: &str, body: &str) -> PathBuf {
        let p = dir.join(name);
        std::fs::write(&p, body).unwrap();
        p
    }

    #[test]
    fn applies_with_git_default() {
        let tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(tmp.path().join(".git")).unwrap();
        let cfg_path = write(tmp.path(), ".mwt.yaml", "repos: [api]\nsetup: []\n");
        let cfg = load(&cfg_path).unwrap();
        assert!(cfg.has_git_at_root);
        assert_eq!(cfg.worktree_path, DEFAULT_WORKTREE_PATH_WITH_GIT);
        assert_eq!(cfg.root, ".");
    }

    #[test]
    fn applies_without_git_default() {
        let tmp = tempfile::tempdir().unwrap();
        let cfg_path = write(tmp.path(), ".mwt.yaml", "repos: [api]\n");
        let cfg = load(&cfg_path).unwrap();
        assert!(!cfg.has_git_at_root);
        assert_eq!(cfg.worktree_path, DEFAULT_WORKTREE_PATH_WITHOUT_GIT);
    }

    #[test]
    fn explicit_worktree_path_preserved() {
        let tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(tmp.path().join(".git")).unwrap();
        let cfg_path = write(
            tmp.path(),
            ".mwt.yaml",
            "worktree_path: \"my-custom/{{REPO}}\"\nrepos: [api]\n",
        );
        let cfg = load(&cfg_path).unwrap();
        // 即使 meta-root 有 .git,显式值不被改写
        assert_eq!(cfg.worktree_path, "my-custom/{{REPO}}");
    }

    #[test]
    fn explicit_worktree_path_no_git_root_preserved() {
        let tmp = tempfile::tempdir().unwrap();
        let cfg_path = write(
            tmp.path(),
            ".mwt.yaml",
            "worktree_path: \".worktrees/{{REPO}}\"\nrepos: [api]\n",
        );
        let cfg = load(&cfg_path).unwrap();
        // 即便默认应为 worktrees/(不带点),显式 .worktrees 保留
        assert_eq!(cfg.worktree_path, ".worktrees/{{REPO}}");
    }

    #[test]
    fn honors_root() {
        let tmp = tempfile::tempdir().unwrap();
        let sub = tmp.path().join("meta");
        std::fs::create_dir_all(&sub).unwrap();
        let cfg_path = write(&sub, ".mwt.yaml", "root: \"../repos\"\nrepos: [api]\n");
        let cfg = load(&cfg_path).unwrap();
        assert_eq!(cfg.root, "../repos");
        // meta_root 应为 tmp/repos 的绝对路径
        assert!(cfg.meta_root.ends_with("repos"));
    }

    #[test]
    fn rejects_empty_repo() {
        let tmp = tempfile::tempdir().unwrap();
        let cfg_path = write(tmp.path(), ".mwt.yaml", "repos: [\"\"]\n");
        let err = load(&cfg_path).unwrap_err();
        assert!(err.to_string().contains("repos[0] is empty"));
    }

    #[test]
    fn rejects_setup_step_two_actions() {
        let tmp = tempfile::tempdir().unwrap();
        let cfg_path = write(
            tmp.path(),
            ".mwt.yaml",
            "repos: [api]\nsetup:\n  - copy: { from: a, to: b }\n    run: { command: x }\n",
        );
        let err = load(&cfg_path).unwrap_err();
        assert!(err.to_string().contains("setup") || err.to_string().contains("action"));
    }

    #[test]
    fn rejects_setup_step_unknown_action() {
        let tmp = tempfile::tempdir().unwrap();
        let cfg_path = write(
            tmp.path(),
            ".mwt.yaml",
            "repos: [api]\nsetup:\n  - mkdir: foo\n",
        );
        let err = load(&cfg_path).unwrap_err();
        assert!(err.to_string().contains("unknown setup action"));
    }

    #[test]
    fn load_from_dir_walks_up() {
        let tmp = tempfile::tempdir().unwrap();
        write(tmp.path(), ".mwt.yaml", "repos: [api]\n");
        let nested = tmp.path().join("a/b/c");
        std::fs::create_dir_all(&nested).unwrap();
        let cfg = load_from_dir(&nested).unwrap();
        assert_eq!(cfg.repos, vec!["api"]);
    }
}
