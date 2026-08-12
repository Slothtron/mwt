//! `mwt add <branch>` — create worktrees for the branch across configured repos.

use std::io::Write;

use clap::Args;

use crate::cli::deps::{Deps, SetupRunner};
use crate::cli::shared::{display_path, max_repo_width, resolve_repo, run_serial, select_repos};

#[derive(Debug, Args)]
pub struct AddOptions {
    /// Subset of config repos (comma-separated or repeated).
    #[arg(long, value_delimiter = ',')]
    pub repos: Vec<String>,

    /// Start point when creating a missing branch (`git worktree add -b`).
    #[arg(long)]
    pub from: String,

    /// Skip setup steps after creating the worktree.
    #[arg(long)]
    pub no_setup: bool,

    /// Best-effort: continue after a repo failure; still exit non-zero if any failed.
    #[arg(long)]
    pub r#continue: bool,
}

pub fn run(
    deps: &Deps,
    branch: &str,
    opts: &AddOptions,
    setup_runner: &mut dyn SetupRunner,
    out: &mut dyn Write,
) -> Result<(), String> {
    if branch.is_empty() {
        return Err("branch is required".to_string());
    }
    let cfg = (deps.load_config)().map_err(|e| format!("load config: {e}"))?;
    let repos = select_repos(&cfg, &opts.repos)?;
    if repos.is_empty() {
        return Err("no repos selected".to_string());
    }

    let setup: &[crate::config::SetupStep] = &cfg.setup;
    let repo_width = max_repo_width(&repos);

    run_serial(&repos, opts.r#continue, |repo| {
        let ctx = resolve_repo(&cfg, repo, branch)?;
        let parent = ctx.worktree_path.parent().ok_or_else(|| {
            format!(
                "worktree path has no parent: {}",
                ctx.worktree_path.display()
            )
        })?;
        (deps.mkdir_all)(parent)
            .map_err(|e| format!("mkdir {parent}: {e}", parent = parent.display()))?;
        deps.git
            .add(&ctx.main_path, &ctx.worktree_path, branch, &opts.from)?;
        if !opts.no_setup {
            setup_runner
                .run(&ctx, setup)
                .map_err(|e| format!("setup: {e}"))?;
        }
        writeln!(
            out,
            "{repo:<repo_width$}  {path}",
            path = display_path(&cfg.meta_root, &ctx.worktree_path.to_string_lossy())
        )
        .map_err(|e| e.to_string())?;
        Ok(())
    })
}
