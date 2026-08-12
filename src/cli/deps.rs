//! Injectable collaborators for CLI subcommands.
//!
//! 具体类型从强耦合的 `git.New()` / `setup.New()` 改为 trait 注入,
//! 方便测试与未来替换(例如 mock 一个不真的调 git 的 `GitClient`)。
//!
//! Rust borrow-checker 不允许 `&mut Box<dyn Write>` 与同一 struct 上
//! 的 `&Box<dyn GitClient>` 字段同时存在(`Box` 内部 trait object
//! 阻挡了 disjoint field borrow 分析)。所以本模块把 `stdout` /
//! `stderr` / `setup` 移出 [`Deps`],由 [`crate::cli`] 的 `dispatch`
//! 在调用子命令前以独立参数传入。`Deps` 本身只持有只读协作者。

use std::io;
use std::path::Path;

use crate::config::Config;
use crate::git::{Adapter, ExecRunner, Worktree};

/// Subset of `git::Adapter` the core subcommands actually use.
pub trait GitClient: Send + Sync {
    fn add(
        &self,
        repo_path: &Path,
        worktree_path: &Path,
        branch: &str,
        from: &str,
    ) -> Result<(), String>;
    fn remove(&self, repo_path: &Path, worktree_path: &Path, force: bool) -> Result<(), String>;
    fn list(&self, repo_path: &Path) -> Result<Vec<Worktree>, String>;
}

/// Subset of `setup::Runner` the CLI uses. Implemented for the real
/// [`crate::setup::Runner`] so the production binary can use it as a
/// `Box<dyn SetupRunner>`.
pub trait SetupRunner: Send {
    fn run(
        &mut self,
        ctx: &crate::pathresolve::Context,
        steps: &[crate::config::SetupStep],
    ) -> Result<(), String>;
}

impl SetupRunner for crate::setup::Runner {
    fn run(
        &mut self,
        ctx: &crate::pathresolve::Context,
        steps: &[crate::config::SetupStep],
    ) -> Result<(), String> {
        crate::setup::Runner::run(self, ctx, steps).map_err(|e| e.to_string())
    }
}

impl<R: crate::git::Runner> GitClient for Adapter<R> {
    fn add(
        &self,
        repo_path: &Path,
        worktree_path: &Path,
        branch: &str,
        from: &str,
    ) -> Result<(), String> {
        Adapter::add(self, repo_path, worktree_path, branch, from).map_err(|e| e.to_string())
    }
    fn remove(&self, repo_path: &Path, worktree_path: &Path, force: bool) -> Result<(), String> {
        Adapter::remove(self, repo_path, worktree_path, force).map_err(|e| e.to_string())
    }
    fn list(&self, repo_path: &Path) -> Result<Vec<Worktree>, String> {
        Adapter::list(self, repo_path).map_err(|e| e.to_string())
    }
}

pub fn default_git() -> Box<dyn GitClient> {
    Box::new(Adapter::<ExecRunner>::new())
}

pub fn default_setup() -> Box<dyn SetupRunner> {
    Box::new(crate::setup::Runner::new())
}

/// `LoadConfig` matches the closure shape used in `Deps` so tests can inject
/// a fixture without touching the filesystem.
pub type LoadConfigFn = dyn Fn() -> Result<Config, String> + Send + Sync;

/// `MkdirAll` matches the closure shape used in `Deps` so tests can inject
/// a no-op or a recording fake.
pub type MkdirAllFn = dyn Fn(&Path) -> io::Result<()> + Send + Sync;

/// Read-only dependencies shared across subcommands. `stdout` / `stderr` /
/// `setup` are *not* here; see the module docs.
pub struct Deps {
    pub git: Box<dyn GitClient>,
    pub load_config: Box<LoadConfigFn>,
    pub mkdir_all: Box<MkdirAllFn>,
}

impl std::fmt::Debug for Deps {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Deps").finish_non_exhaustive()
    }
}

impl Deps {
    /// Default dependencies (real OS + real git + real config loader).
    pub fn real() -> Self {
        Self {
            git: default_git(),
            load_config: Box::new(|| {
                let wd = std::env::current_dir().map_err(|e| e.to_string())?;
                crate::config::load_from_dir(&wd).map_err(|e| e.to_string())
            }),
            mkdir_all: Box::new(|p| std::fs::create_dir_all(p)),
        }
    }
}
