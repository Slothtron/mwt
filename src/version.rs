//! Binary version metadata.
//!
//! 与 Go 版 `mwt-legacy/internal/version/version.go` 行为一致。
//! 1. `CARGO_PKG_VERSION` 是 version 来源。
//! 2. Commit / build date 由 build script 注入;无 build script 时回退到
//!    编译期常量,运行时无法 fallback(Go 用 `debug.ReadBuildInfo`;Rust
//!    不再有运行时 build info,故保持简单的 `dev`/`none` 默认)。

use std::fmt;

/// Package-level version string. Set at build time via `CARGO_PKG_VERSION`.
pub const VERSION: &str = env!("CARGO_PKG_VERSION");

/// Short git commit hash, or `"none"` if not built from a git checkout.
/// Set at build time via `MWT_COMMIT` env (build.rs is the usual source).
pub const COMMIT: &str = match option_env!("MWT_COMMIT") {
    Some(s) if !s.is_empty() => s,
    _ => "none",
};

/// Build date, or `"unknown"`.
pub const BUILD_DATE: &str = match option_env!("MWT_BUILD_DATE") {
    Some(s) if !s.is_empty() => s,
    _ => "unknown",
};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Info {
    pub version: &'static str,
    pub commit: &'static str,
    pub build_date: &'static str,
}

impl Info {
    pub const fn current() -> Self {
        Self {
            version: VERSION,
            commit: COMMIT,
            build_date: BUILD_DATE,
        }
    }
}

impl fmt::Display for Info {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "mwt version {} (commit {}, built {})",
            self.version, self.commit, self.build_date
        )
    }
}

/// Convenience: the current version formatted as a single line.
pub fn string() -> String {
    Info::current().to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn current_is_populated() {
        let i = Info::current();
        assert!(!i.version.is_empty());
        // commit / build_date fall back to "none" / "unknown" when unbuilt.
        assert!(!i.commit.is_empty());
        assert!(!i.build_date.is_empty());
    }

    #[test]
    fn string_format_includes_three_fields() {
        let s = string();
        assert!(s.starts_with("mwt version "));
        assert!(s.contains("(commit "));
        assert!(s.contains(", built "));
    }
}
