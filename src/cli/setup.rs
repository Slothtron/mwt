//! `mwt setup <branch>` — re-run configured setup steps on existing worktrees.

use std::io::Write;

use clap::Args;

use crate::cli::deps::{Deps, SetupRunner};
use crate::cli::shared::{display_path, max_repo_width, resolve_repo, run_serial, select_repos};

#[derive(Debug, Args)]
pub struct SetupOptions {
    #[arg(long, value_delimiter = ',')]
    pub repos: Vec<String>,

    #[arg(long)]
    pub r#continue: bool,
}

pub fn run(
    deps: &Deps,
    branch: &str,
    opts: &SetupOptions,
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
        require_worktree_dir(&ctx.worktree_path)?;
        setup_runner
            .run(&ctx, setup)
            .map_err(|e| format!("setup: {e}"))?;
        writeln!(
            out,
            "{repo:<repo_width$}  {path}",
            path = display_path(&cfg.meta_root, &ctx.worktree_path.to_string_lossy())
        )
        .map_err(|e| e.to_string())?;
        Ok(())
    })
}

fn require_worktree_dir(path: &std::path::Path) -> Result<(), String> {
    match std::fs::symlink_metadata(path) {
        Ok(m) if m.is_dir() => Ok(()),
        Ok(_) => Err(format!(
            "worktree path is not a directory: {}",
            path.display()
        )),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Err(format!(
            "worktree does not exist: {} (create it with mwt add first)",
            path.display()
        )),
        Err(e) => Err(format!("stat worktree {}: {e}", path.display())),
    }
}
