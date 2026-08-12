//! Compare disk state with git worktree registrations (§5.3).
//!
//! 只报告 findings + 修复建议,**不**删除任何路径。

use std::collections::{HashMap, HashSet};
use std::fmt;
use std::io;
use std::path::{Path, PathBuf};

use thiserror::Error;

use crate::config::Config;
use crate::git::Worktree;
use crate::pathresolve::{Context, Stage};

pub mod format;
pub mod scan;

pub use format::format_report;
pub use scan::DiskWorktree;

/// Kind of one doctor finding. Sorted alphabetically when reporting.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum Kind {
    MainMissing,
    Prunable,
    RootMissing,
    SetupMissing,
    Unregistered,
}

impl Kind {
    pub fn as_str(self) -> &'static str {
        match self {
            Kind::MainMissing => "main_missing",
            Kind::Prunable => "prunable",
            Kind::RootMissing => "root_missing",
            Kind::SetupMissing => "setup_missing",
            Kind::Unregistered => "unregistered",
        }
    }
}

impl fmt::Display for Kind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

/// One reportable issue with optional repair suggestions.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Finding {
    pub kind: Kind,
    pub repo: String,
    pub branch: String,
    pub path: String,
    pub message: String,
    pub suggest: Vec<String>,
}

impl Finding {
    pub fn new(
        kind: Kind,
        repo: impl Into<String>,
        branch: impl Into<String>,
        path: impl Into<String>,
        message: impl Into<String>,
    ) -> Self {
        Self {
            kind,
            repo: repo.into(),
            branch: branch.into(),
            path: path.into(),
            message: message.into(),
            suggest: Vec::new(),
        }
    }
    pub fn with_suggest(mut self, s: impl IntoIterator<Item = impl Into<String>>) -> Self {
        self.suggest = s.into_iter().map(Into::into).collect();
        self
    }
}

/// Adapter for the git operations doctor needs.
pub trait GitLister: Send + Sync {
    fn list(&self, repo_path: &Path) -> Result<Vec<Worktree>, GitListError>;
}

#[derive(Debug, Error)]
pub enum GitListError {
    #[error("git list: {0}")]
    Other(String),
}

impl<T: crate::git::Runner> GitLister for crate::git::Adapter<T> {
    fn list(&self, repo_path: &Path) -> Result<Vec<Worktree>, GitListError> {
        self.list(repo_path)
            .map_err(|e| GitListError::Other(e.to_string()))
    }
}

/// Filesystem surface doctor needs (injectable for tests).
/// `stat` returns `Ok(())` if the path exists, `Err(NotFound)` otherwise.
pub trait Fs: Send + Sync {
    fn stat(&self, name: &Path) -> io::Result<()>;
    fn read_dir(&self, name: &Path) -> io::Result<Vec<DirEntry>>;
}

/// Minimal `DirEntry` for the FS trait.
#[derive(Debug, Clone)]
pub struct DirEntry {
    pub name: String,
    pub is_dir: bool,
}

/// Real OS filesystem.
#[derive(Debug, Default, Clone, Copy)]
pub struct OsFs;

impl Fs for OsFs {
    fn stat(&self, name: &Path) -> io::Result<()> {
        std::fs::symlink_metadata(name).map(|_| ())
    }
    fn read_dir(&self, name: &Path) -> io::Result<Vec<DirEntry>> {
        let mut out = Vec::new();
        for entry in std::fs::read_dir(name)? {
            let e = entry?;
            let ft = e.file_type().ok();
            let is_dir = ft.as_ref().map(|t| t.is_dir()).unwrap_or(false);
            out.push(DirEntry {
                name: e.file_name().to_string_lossy().to_string(),
                is_dir,
            });
        }
        Ok(out)
    }
}

/// Checker runs §5.3 inspections.
pub struct Checker<'a> {
    pub git: &'a dyn GitLister,
    pub fs: &'a dyn Fs,
}

impl<'a> Checker<'a> {
    pub fn new(git: &'a dyn GitLister, fs: &'a dyn Fs) -> Self {
        Self { git, fs }
    }

    pub fn check(&self, cfg: &Config) -> Result<Vec<Finding>, DoctorError> {
        if cfg.meta_root.as_os_str().is_empty() {
            return Err(DoctorError::Empty("meta root".into()));
        }

        if let Err(e) = self.fs.stat(&cfg.meta_root) {
            if e.kind() == io::ErrorKind::NotFound {
                return Ok(vec![
                    Finding::new(
                        Kind::RootMissing,
                        "",
                        "",
                        cfg.meta_root.to_string_lossy().to_string(),
                        format!("meta root missing: {}", cfg.meta_root.display()),
                    )
                    .with_suggest(["# fix root in .mwt.yaml or create the meta-root directory"]),
                ]);
            }
            return Err(DoctorError::StatMeta {
                path: cfg.meta_root.clone(),
                source: e,
            });
        }

        let mut findings: Vec<Finding> = Vec::new();
        for repo in &cfg.repos {
            findings.extend(self.check_repo(cfg, repo)?);
        }

        findings.sort_by(|a, b| {
            a.kind
                .cmp(&b.kind)
                .then_with(|| a.repo.cmp(&b.repo))
                .then_with(|| a.branch.cmp(&b.branch))
                .then_with(|| a.path.cmp(&b.path))
        });
        Ok(findings)
    }

    fn check_repo(&self, cfg: &Config, repo: &str) -> Result<Vec<Finding>, DoctorError> {
        let main = cfg.meta_root.join(repo);
        if let Err(e) = self.fs.stat(&main) {
            if e.kind() == io::ErrorKind::NotFound {
                return Ok(vec![Finding::new(
                    Kind::MainMissing,
                    repo,
                    "",
                    main.to_string_lossy().to_string(),
                    format!("main checkout missing: {}", main.display()),
                )
                .with_suggest(["# fix .mwt.yaml repos entry or place the git checkout at the expected path"])]);
            }
            return Err(DoctorError::StatMain {
                path: main,
                source: e,
            });
        }

        let registered = self
            .git
            .list(&main)
            .map_err(|e| DoctorError::ListWorktrees {
                repo: repo.to_string(),
                source: e,
            })?;

        let mut reg_by_path: HashMap<PathBuf, Worktree> = HashMap::new();
        for wt in registered {
            if wt.bare || wt.path.is_empty() {
                continue;
            }
            reg_by_path.insert(clean_path(&PathBuf::from(&wt.path)), wt);
        }

        let mut findings: Vec<Finding> = Vec::new();

        // Prunable
        for (path, wt) in &reg_by_path {
            if path == &clean_path(&main) {
                continue;
            }
            match self.fs.stat(path) {
                Ok(()) => continue,
                Err(e) if e.kind() == io::ErrorKind::NotFound => {}
                Err(e) => {
                    return Err(DoctorError::StatWorktree {
                        path: path.clone(),
                        source: e,
                    });
                }
            }

            let branch = wt.branch.clone();
            let canonical = if !branch.is_empty() {
                Context::resolve_from_config(cfg, repo, &branch)
                    .ok()
                    .map(|c| c.worktree_path.to_string_lossy().to_string())
                    .unwrap_or_default()
            } else {
                String::new()
            };

            let mut suggest: Vec<String> = Vec::new();
            suggest.push(format!("git -C {} worktree prune", main.display()));
            if !branch.is_empty() {
                suggest.push(format!("mwt add {branch} --repos {repo}"));
                if !canonical.is_empty() {
                    suggest.push(format!("# canonical path from worktree_path: {canonical}"));
                }
            } else {
                suggest.push(format!(
                    "# re-add via mwt add <branch> --repos {repo} after identifying the branch"
                ));
            }

            findings.push(
                Finding::new(
                    Kind::Prunable,
                    repo.to_string(),
                    branch.clone(),
                    path.to_string_lossy().to_string(),
                    format!(
                        "registered worktree path missing (prunable): {}",
                        path.display()
                    ),
                )
                .with_suggest(suggest),
            );
        }

        // Unregistered
        let disk_entries =
            scan::discover_disk_worktrees(self.fs, &cfg.meta_root, &cfg.worktree_path, repo)?;
        for d in disk_entries {
            if reg_by_path.contains_key(&PathBuf::from(clean_str(&d.path))) {
                continue;
            }
            findings.push(
                Finding::new(
                    Kind::Unregistered,
                    repo.to_string(),
                    d.branch.clone(),
                    d.path.clone(),
                    format!(
                        "disk directory exists but is not a registered worktree: {}",
                        d.path
                    ),
                )
                .with_suggest([
                    format!("mwt add {} --repos {repo}", d.branch),
                    "# or remove the unregistered directory manually (mwt will not auto-delete)"
                        .to_string(),
                ]),
            );
        }

        // Setup copy destinations missing
        if !cfg.setup.is_empty() {
            for (path, wt) in &reg_by_path {
                if path == &clean_path(&main) {
                    continue;
                }
                if self.fs.stat(path).is_err() {
                    continue;
                }
                let branch = wt.branch.clone();
                if branch.is_empty() {
                    continue;
                }
                let mut ctx = Context::resolve_from_config(cfg, repo, &branch)?;
                if clean_str(&ctx.worktree_path.to_string_lossy()) != path.to_string_lossy() {
                    ctx.worktree_path = path.clone();
                    ctx.worktree_name = path
                        .file_name()
                        .map(|s| s.to_string_lossy().to_string())
                        .unwrap_or_default();
                }
                findings.extend(self.check_setup_copies(cfg, &ctx)?);
            }
        }

        Ok(findings)
    }

    fn check_setup_copies(&self, cfg: &Config, ctx: &Context) -> Result<Vec<Finding>, DoctorError> {
        let mut findings: Vec<Finding> = Vec::new();
        let mut seen_to: HashSet<String> = HashSet::new();

        for step in &cfg.setup {
            let copy = match &step.copy {
                Some(c) => c,
                None => continue,
            };
            let from_raw =
                ctx.expand(&copy.from, Stage::Setup)
                    .map_err(|e| DoctorError::ExpandField {
                        field: "copy.from",
                        source: Box::new(e),
                    })?;
            let to_raw =
                ctx.expand(&copy.to, Stage::Setup)
                    .map_err(|e| DoctorError::ExpandField {
                        field: "copy.to",
                        source: Box::new(e),
                    })?;
            let from = abs_or_join(&from_raw, &ctx.root);
            let to = abs_or_join(&to_raw, &ctx.worktree_path);

            if seen_to.contains(&to) {
                continue;
            }
            match self.fs.stat(Path::new(&to)) {
                Ok(()) => {
                    seen_to.insert(to);
                    continue;
                }
                Err(e) if e.kind() == io::ErrorKind::NotFound => {}
                Err(e) => {
                    return Err(DoctorError::StatSetupDest {
                        path: PathBuf::from(to),
                        source: e,
                    });
                }
            }

            if copy.skip_if_missing_src_or_default() {
                match self.fs.stat(Path::new(&from)) {
                    Err(e) if e.kind() == io::ErrorKind::NotFound => continue,
                    Err(e) => {
                        return Err(DoctorError::StatSetupSrc {
                            path: PathBuf::from(from),
                            source: e,
                        });
                    }
                    Ok(_) => {}
                }
            }

            seen_to.insert(to.clone());
            findings.push(
                Finding::new(
                    Kind::SetupMissing,
                    ctx.repo.clone(),
                    ctx.branch.clone(),
                    to.clone(),
                    format!("setup copy destination missing: {to}"),
                )
                .with_suggest([format!("mwt setup {} --repos {}", ctx.branch, ctx.repo)]),
            );
        }
        Ok(findings)
    }
}

#[derive(Debug, Error)]
pub enum DoctorError {
    #[error("doctor: {0} is empty")]
    Empty(String),
    #[error("doctor: stat meta root {path}: {source}", path = .path.display())]
    StatMeta {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("doctor: stat main {path}: {source}", path = .path.display())]
    StatMain {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("doctor: list worktrees for {repo}: {source}")]
    ListWorktrees {
        repo: String,
        #[source]
        source: GitListError,
    },
    #[error("doctor: stat worktree {path}: {source}", path = .path.display())]
    StatWorktree {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("doctor: stat setup dest {path}: {source}", path = .path.display())]
    StatSetupDest {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("doctor: stat setup src {path}: {source}", path = .path.display())]
    StatSetupSrc {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("doctor: expand {field}: {source}")]
    ExpandField {
        field: &'static str,
        #[source]
        source: Box<crate::pathresolve::ExpandError>,
    },
    #[error("doctor: pathresolve: {0}")]
    PathResolve(#[from] crate::pathresolve::ResolveError),
    #[error("doctor: scan: {0}")]
    Scan(#[from] scan::ScanError),
}

fn clean_path(p: &Path) -> PathBuf {
    let mut out = PathBuf::new();
    for c in p.components() {
        match c {
            std::path::Component::ParentDir => {
                out.pop();
            }
            std::path::Component::CurDir => {}
            other => out.push(other.as_os_str()),
        }
    }
    out
}

fn clean_str(p: &str) -> String {
    clean_path(&PathBuf::from(p)).to_string_lossy().to_string()
}

fn abs_or_join(path: &str, base: &Path) -> String {
    let p = Path::new(path);
    let out = if p.is_absolute() {
        clean_path(p)
    } else {
        clean_path(&base.join(p))
    };
    out.to_string_lossy().to_string()
}
