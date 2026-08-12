//! `mwt rm <branch>` — remove worktrees for the branch across configured repos.

use std::io::Write;

use clap::Args;

use crate::cli::deps::Deps;
use crate::cli::shared::{display_path, max_repo_width, resolve_repo, run_serial, select_repos};

#[derive(Debug, Args)]
pub struct RmOptions {
    #[arg(long, value_delimiter = ',')]
    pub repos: Vec<String>,

    /// Pass `--force` to `git worktree remove` (dirty/locked/residual).
    #[arg(long)]
    pub force: bool,

    /// Best-effort: continue after a repo failure; still exit non-zero if any failed.
    #[arg(long)]
    pub r#continue: bool,
}

pub fn run(deps: &Deps, branch: &str, opts: &RmOptions, out: &mut dyn Write) -> Result<(), String> {
    if branch.is_empty() {
        return Err("branch is required".to_string());
    }
    let cfg = (deps.load_config)().map_err(|e| format!("load config: {e}"))?;
    let repos = select_repos(&cfg, &opts.repos)?;
    if repos.is_empty() {
        return Err("no repos selected".to_string());
    }

    let repo_width = max_repo_width(&repos);

    run_serial(&repos, opts.r#continue, |repo| {
        let ctx = resolve_repo(&cfg, repo, branch)?;
        deps.git
            .remove(&ctx.main_path, &ctx.worktree_path, opts.force)?;
        writeln!(
            out,
            "{repo:<repo_width$}  {path}",
            path = display_path(&cfg.meta_root, &ctx.worktree_path.to_string_lossy())
        )
        .map_err(|e| e.to_string())?;
        Ok(())
    })
}
