//! `mwt init` — scan for git checkouts and write `.mwt.yaml` in cwd.

use std::io::Write;
use std::path::Path;

use clap::Args;

use crate::cli::deps::Deps;
use crate::config::{
    DEFAULT_WORKTREE_PATH_WITH_GIT, DEFAULT_WORKTREE_PATH_WITHOUT_GIT, git_exists_at,
};
use crate::initscan;

const DEFAULT_INIT_DEPTH: usize = 10;

#[derive(Debug, Args)]
pub struct InitOptions {
    /// Max directory depth to scan (0 = current directory only).
    #[arg(long, default_value_t = DEFAULT_INIT_DEPTH)]
    pub depth: usize,

    /// Overwrite existing `.mwt.yaml`.
    #[arg(long)]
    pub force: bool,

    /// Print the config that would be written without creating the file.
    #[arg(long)]
    pub dry_run: bool,
}

pub fn run(deps: &Deps, opts: &InitOptions, out: &mut dyn Write) -> Result<(), String> {
    if opts.depth > i32::MAX as usize {
        return Err(format!("depth must fit in i32: {}", opts.depth));
    }

    let wd = std::env::current_dir().map_err(|e| e.to_string())?;
    let repos = initscan::discover(&wd, opts.depth).map_err(|e| e.to_string())?;
    if repos.is_empty() {
        return Err(format!(
            "no git repositories found under {} (depth={})",
            wd.display(),
            opts.depth
        ));
    }

    let worktree_path = if git_exists_at(&wd) {
        DEFAULT_WORKTREE_PATH_WITH_GIT.to_string()
    } else {
        DEFAULT_WORKTREE_PATH_WITHOUT_GIT.to_string()
    };

    let yaml_text = render_init_yaml(&wd, &worktree_path, &repos)?;
    let out_path = wd.join(".mwt.yaml");

    if opts.dry_run {
        writeln!(out, "# would write {}", out_path.display()).map_err(|e| e.to_string())?;
        out.write_all(yaml_text.as_bytes())
            .map_err(|e| e.to_string())?;
        return Ok(());
    }

    if !opts.force && out_path.exists() {
        return Err(format!(
            "{} already exists (use --force to overwrite)",
            out_path.display()
        ));
    }

    std::fs::write(&out_path, yaml_text)
        .map_err(|e| format!("write {}: {e}", out_path.display()))?;

    writeln!(out, "wrote {} ({} repos)", out_path.display(), repos.len())
        .map_err(|e| e.to_string())?;
    writeln!(out, "worktree_path: {worktree_path}").map_err(|e| e.to_string())?;
    for r in &repos {
        writeln!(out, "  - {r}").map_err(|e| e.to_string())?;
    }
    // `deps` is held only to keep the same call surface as other subcommands
    // (potential future "init --from-stdin" or similar). Avoid an unused warning.
    let _ = deps;
    Ok(())
}

/// Render the YAML body of `.mwt.yaml`. Centralized so unit tests can verify
/// the produced string without touching the filesystem.
fn render_init_yaml(_wd: &Path, worktree_path: &str, repos: &[String]) -> Result<String, String> {
    // Round-trip through a serde-friendly shape to keep the on-disk format
    // stable and free of derived fields.
    let wire = InitWire {
        root: ".".to_string(),
        worktree_path: worktree_path.to_string(),
        repos: repos.to_vec(),
        setup: Vec::new(),
    };
    serde_saphyr::to_string(&wire).map_err(|e| format!("encode .mwt.yaml: {e}"))
}

#[derive(Debug, serde::Serialize)]
struct InitWire {
    root: String,
    worktree_path: String,
    repos: Vec<String>,
    setup: Vec<InitStepWire>,
}

#[derive(Debug, serde::Serialize)]
struct InitStepWire {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn render_uses_canonical_field_order() {
        let yaml = render_init_yaml(
            Path::new("/tmp/x"),
            DEFAULT_WORKTREE_PATH_WITH_GIT,
            &["a".to_string(), "b/c".to_string()],
        )
        .unwrap();
        // serde-saphyr's block format puts list items at column 0 and quotes
        // bare dots; we assert field names and values without binding
        // specific whitespace.
        assert!(yaml.contains("root:"));
        assert!(yaml.contains("worktree_path: .worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}"));
        assert!(yaml.contains("repos:"));
        assert!(yaml.lines().any(|l| l.trim() == "- a"));
        assert!(yaml.lines().any(|l| l.trim() == "- b/c"));
        assert!(yaml.contains("setup: []"));
    }

    #[test]
    fn load_from_dir_accepts_our_render() {
        let tmp = tempfile::tempdir().unwrap();
        let body = render_init_yaml(
            tmp.path(),
            DEFAULT_WORKTREE_PATH_WITHOUT_GIT,
            &["api".to_string()],
        )
        .unwrap();
        std::fs::write(tmp.path().join(".mwt.yaml"), body).unwrap();
        let cfg = crate::config::load_from_dir(tmp.path()).unwrap();
        assert_eq!(cfg.repos, vec!["api".to_string()]);
        assert_eq!(cfg.worktree_path, DEFAULT_WORKTREE_PATH_WITHOUT_GIT);
    }
}
