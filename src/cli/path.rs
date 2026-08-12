//! `mwt path <branch> <repo>` — print the absolute worktree path.

use std::io::Write;

use clap::Args;

use crate::cli::deps::Deps;
use crate::cli::format::{OutputFormat, write_path};
use crate::cli::shared::{resolve_repo, select_repos};

#[derive(Debug, Args)]
pub struct PathOptions {
    /// Output format: `table` (default) or `json`.
    #[arg(long, value_enum, default_value_t = OutputFormat::Table)]
    pub format: OutputFormat,
}

pub fn run(
    deps: &Deps,
    branch: &str,
    repo: &str,
    opts: &PathOptions,
    out: &mut dyn Write,
) -> Result<(), String> {
    if branch.is_empty() {
        return Err("branch is required".to_string());
    }
    if repo.is_empty() {
        return Err("repo is required".to_string());
    }
    let cfg = (deps.load_config)().map_err(|e| format!("load config: {e}"))?;
    // Validate repo is in config (path is still deterministic from template).
    select_repos(&cfg, std::slice::from_ref(&repo.to_string()))?;
    let ctx = resolve_repo(&cfg, repo, branch)?;
    write_path(out, opts.format, &ctx.worktree_path.to_string_lossy())
        .map_err(|e| e.to_string())?;
    Ok(())
}
