//! mwt — manage polyrepo Git worktrees from a single `.mwt.yaml`.
//!
//! 模块树:
//! - [`config`] — `.mwt.yaml` 加载/查找 + `SetupStep` 单键契约
//! - [`pathresolve`] — 路径占位符 `{{NAME}}` 渲染
//! - [`git`] — `std::process::Command` 调系统 `git`
//! - [`setup`] — `copy` + `run` 步骤执行
//! - [`doctor`] — §5.3 inspector
//! - [`initscan`] — 递归发现 `.git` 目录
//! - [`version`] — build metadata
//! - [`cli`] — clap 派生根 + 8 个子命令调度
//!
//! 公开入口 [`run`] 是 CLI 调度入口(供 `main.rs` 与集成测试共用)。

#![warn(clippy::all)]

use std::io;
use std::process::ExitCode;

pub mod cli;
pub mod config;
pub mod doctor;
pub mod git;
pub mod initscan;
pub mod pathresolve;
pub mod setup;
pub mod version;

/// Build metadata, mirror of `mwt-legacy/internal/version`.
pub fn version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

/// Top-level dispatch used by [`main`](crate::main) and integration tests.
pub fn run(cli: cli::Cli) -> ExitCode {
    let stdout = io::stdout();
    let stderr = io::stderr();
    let mut setup = cli::deps::default_setup();
    let deps = cli::deps::Deps::real();
    let mut stdout = stdout;
    let mut stderr = stderr;
    cli::run(cli, &mut stdout, &mut stderr, setup.as_mut(), deps)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn version_is_set() {
        assert!(!version().is_empty());
    }
}
