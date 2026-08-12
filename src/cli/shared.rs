//! Cross-command helpers shared by the CLI subcommand implementations.

use std::path::{Path, PathBuf};

use crate::config::Config;
use crate::pathresolve;

/// `selectRepos`: pick the subset of `cfg.repos` named in `requested`.
/// Empty `requested` → all repos. Unknown names → error.
pub fn select_repos(cfg: &Config, requested: &[String]) -> Result<Vec<String>, String> {
    let known: std::collections::HashSet<&str> = cfg.repos.iter().map(String::as_str).collect();
    let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();
    let mut out: Vec<String> = Vec::new();
    for r in requested {
        let r = r.trim();
        if r.is_empty() {
            return Err("empty repo name in --repos".to_string());
        }
        if !known.contains(r) {
            return Err(format!("repo {r:?} is not in config repos"));
        }
        if seen.insert(r.to_string()) {
            out.push(r.to_string());
        }
    }
    if out.is_empty() {
        Ok(cfg.repos.clone())
    } else {
        Ok(out)
    }
}

/// `runSerial`: invoke `fn` for each repo in order.
/// `continue_on_err=true` keeps going and joins all errors at the end.
pub fn run_serial<F>(repos: &[String], continue_on_err: bool, mut fn_: F) -> Result<(), String>
where
    F: FnMut(&str) -> Result<(), String>,
{
    let mut errs: Vec<String> = Vec::new();
    for repo in repos {
        if let Err(e) = fn_(repo) {
            let wrapped = format!("{repo}: {e}");
            if !continue_on_err {
                return Err(wrapped);
            }
            errs.push(wrapped);
        }
    }
    if errs.is_empty() {
        Ok(())
    } else {
        Err(errs.join("; "))
    }
}

pub fn resolve_repo(
    cfg: &Config,
    repo: &str,
    branch: &str,
) -> Result<pathresolve::Context, String> {
    pathresolve::Context::resolve_from_config(cfg, repo, branch).map_err(|e| e.to_string())
}

pub fn main_path(cfg: &Config, repo: &str) -> PathBuf {
    cfg.meta_root.join(repo)
}

/// `displayPath`: prefer path relative to `meta_root`; fall back to absolute
/// when the path escapes `meta_root` or the relative computation fails.
pub fn display_path(meta_root: &Path, abs: &str) -> String {
    if meta_root.as_os_str().is_empty() || abs.is_empty() {
        return abs.to_string();
    }
    let abs_p = Path::new(abs);
    match abs_p.strip_prefix(meta_root) {
        Ok(rel) => {
            let s = rel.to_string_lossy();
            if s.is_empty() {
                return ".".to_string();
            }
            s.replace(std::path::MAIN_SEPARATOR, "/")
        }
        Err(_) => abs.to_string(),
    }
}

pub fn max_repo_width(repos: &[String]) -> usize {
    repos.iter().map(String::len).max().unwrap_or(0)
}
