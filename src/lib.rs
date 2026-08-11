//! mwt — manage polyrepo Git worktrees from a single `.mwt.yaml`.
//!
//! 本 lib 暂未提供公开 API(阶段 B 起的 config / pathresolve / git / setup /
//! doctor / initscan / cli / version 各模块落地后,在此 re-export)。
//!
//! 阶段 E 完成后,`run()` 是 CLI 调度入口。

#![warn(clippy::all)]

pub mod config;
pub mod doctor;
pub mod git;
pub mod initscan;
pub mod pathresolve;
pub mod setup;
pub mod version;

pub fn version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn version_is_set() {
        assert!(!version().is_empty());
    }
}
