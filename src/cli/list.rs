//! `mwt list` — aggregate git worktree list from each configured repo.

use std::io::Write;

use clap::Args;

use crate::cli::deps::Deps;
use crate::cli::format::{ListRow, OutputFormat, write_list};
use crate::cli::shared::{display_path, main_path, run_serial, select_repos};

#[derive(Debug, Args)]
pub struct ListOptions {
    #[arg(long, value_delimiter = ',')]
    pub repos: Vec<String>,

    /// Filter by branch short name (accepts accidental `refs/heads/` prefix).
    #[arg(long, default_value = "")]
    pub branch: String,

    #[arg(long)]
    pub r#continue: bool,

    /// Output format: `table` (default) or `json`.
    #[arg(long, value_enum, default_value_t = OutputFormat::Table)]
    pub format: OutputFormat,
}

pub fn run(deps: &Deps, opts: &ListOptions, out: &mut dyn Write) -> Result<(), String> {
    let cfg = (deps.load_config)().map_err(|e| format!("load config: {e}"))?;
    let repos = select_repos(&cfg, &opts.repos)?;

    let mut rows: Vec<ListRow> = Vec::new();
    let err = run_serial(&repos, opts.r#continue, |repo| {
        let main = main_path(&cfg, repo);
        let wts = deps.git.list(&main)?;
        for wt in wts {
            if wt.bare {
                continue;
            }
            let mut branch = wt.branch;
            if branch.is_empty() && wt.detached {
                branch = "(detached)".to_string();
            }
            if !opts.branch.is_empty() && !branch_matches(&branch, &opts.branch) {
                continue;
            }
            rows.push(ListRow {
                repo: repo.to_string(),
                branch,
                path: display_path(&cfg.meta_root, &wt.path),
            });
        }
        Ok(())
    });

    write_list(out, opts.format, &rows).map_err(|e| e.to_string())?;
    err
}

fn branch_matches(actual: &str, want: &str) -> bool {
    fn strip(s: &str) -> &str {
        s.strip_prefix("refs/heads/").unwrap_or(s)
    }
    strip(actual) == strip(want)
}
