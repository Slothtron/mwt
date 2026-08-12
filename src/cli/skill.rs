//! `mwt skill install` — Install the mwt Agent skill from
//! repository-side `skills/mwt/` to the user skills directory.
//!
//! Pure-Rust reimplementation of `scripts/install-skill.sh`,
//! sharing the same behavior contract: look up source, verify,
//! copy to dest with existence/force/dry-run guards.
//!
//! No `include_str!`, no embedding, no shell out.

use std::io::Write;
use std::path::{Path, PathBuf};
use std::{env, fs};

use crate::cli::format::OutputFormat;

// ---------------------------------------------------------------------------
// CLI options
// ---------------------------------------------------------------------------

#[derive(Debug, clap::Parser)]
pub struct SkillInstallOptions {
    /// Parent skills directory (default: $HOME/.agents/skills).
    #[arg(long)]
    pub dir: Option<String>,

    /// Path to source skills/mwt directory.
    #[arg(long)]
    pub source: Option<String>,

    /// Overwrite an existing installation.
    #[arg(long, default_value_t = false)]
    pub force: bool,

    /// Print the plan without making any changes.
    #[arg(long, default_value_t = false)]
    pub dry_run: bool,

    /// Output format (table | json).
    #[arg(long, value_enum, default_value_t = OutputFormat::Table)]
    pub format: OutputFormat,
}

// ---------------------------------------------------------------------------
// Subcommand enum (allows future `mwt skill <sub>` extensions)
// ---------------------------------------------------------------------------

#[derive(Debug, clap::Subcommand)]
pub enum SkillCommand {
    /// Install the mwt Agent skill from repository-side assets.
    Install(SkillInstallOptions),
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

/// Resolve source `skills/mwt/` directory via lookup chain:
/// 1. `--source` / `--source=` CLI
/// 2. `MWT_SKILL_SOURCE` env var
/// 3. `./skills/mwt` relative to cwd
fn resolve_source(cli_source: Option<&str>) -> Result<PathBuf, String> {
    // 1. explicit CLI arg
    if let Some(s) = cli_source {
        return Ok(PathBuf::from(s));
    }
    // 2. env var
    if let Ok(v) = env::var("MWT_SKILL_SOURCE") {
        return Ok(PathBuf::from(v));
    }
    // 3. cwd ./skills/mwt
    let cwd = env::current_dir().map_err(|e| format!("cannot read cwd: {e}"))?;
    let candidate = cwd.join("skills/mwt");
    if candidate.join("SKILL.md").is_file() {
        return Ok(candidate);
    }

    Err(format!(
        "source skill not found.\nLooked at: {}/skills/mwt/\n\
         Specify --source <path>, set MWT_SKILL_SOURCE env, or run from the repository root.",
        cwd.display()
    ))
}

/// Resolve destination parent directory followed by `append` (default: "mwt").
fn resolve_dest(cli_dir: Option<&str>, append: &str) -> Result<PathBuf, String> {
    let base = if let Some(d) = cli_dir {
        d.to_owned()
    } else if let Ok(v) = env::var("MWT_SKILL_DIR") {
        v
    } else {
        let home = env::var("HOME").map_err(|_| {
            "HOME is unset and neither --dir nor MWT_SKILL_DIR provided".to_string()
        })?;
        format!("{home}/.agents/skills")
    };

    let p = if base.starts_with('/') {
        PathBuf::from(base)
    } else {
        let cwd = env::current_dir().map_err(|e| format!("cannot read cwd: {e}"))?;
        cwd.join(&base)
    };
    Ok(p.join(append))
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

pub fn run(opts: &SkillInstallOptions, out: &mut dyn Write) -> Result<(), String> {
    let source = resolve_source(opts.source.as_deref())?;
    verify_source(&source)?;

    let dest = resolve_dest(opts.dir.as_deref(), "mwt")?;

    // --- existence checks (same as install-skill.sh) ---
    if dest.exists() {
        if !dest.is_dir() {
            return Err(format!("{} exists and is not a directory", dest.display()));
        }
        if !opts.force {
            return Err(format!(
                "{} already exists (use --force to overwrite)",
                dest.display()
            ));
        }
        if !opts.dry_run {
            fs::remove_dir_all(&dest)
                .map_err(|e| format!("cannot remove {}: {e}", dest.display()))?;
        }
    }

    // --- dry-run: print plan ---
    if opts.dry_run {
        write_dry_run(out, opts.format, &source, &dest)?;
        return Ok(());
    }

    // --- copy ---
    copy_skill_tree(&source, &dest)?;

    // --- success message ---
    match opts.format {
        OutputFormat::Table => {
            writeln!(out, "synced skill to {}", dest.display()).map_err(|e| e.to_string())?;
        }
        OutputFormat::Json => {
            let v = serde_json::json!({
                "action": "installed",
                "dest": dest.display().to_string(),
            });
            let s = serde_json::to_string_pretty(&v).map_err(|e| e.to_string())?;
            writeln!(out, "{s}").map_err(|e| e.to_string())?;
        }
    }

    Ok(())
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

fn verify_source(source: &Path) -> Result<(), String> {
    if !source.is_dir() {
        return Err(format!("source skill not found at {}", source.display()));
    }
    if !source.join("SKILL.md").is_file() {
        return Err(format!("SKILL.md missing in {}", source.display()));
    }
    Ok(())
}

fn write_dry_run(
    out: &mut dyn Write,
    fmt: OutputFormat,
    source: &Path,
    dest: &Path,
) -> Result<(), String> {
    let entries = list_entries(source)?;
    match fmt {
        OutputFormat::Table => {
            writeln!(out, "plan: mkdir -p {}", dest.display()).map_err(|e| e.to_string())?;
            for e in &entries {
                writeln!(out, "plan: copy {} -> {}/", e.display(), dest.display())
                    .map_err(|e| e.to_string())?;
            }
        }
        OutputFormat::Json => {
            let entry_strs: Vec<String> = entries.iter().map(|e| e.display().to_string()).collect();
            let v = serde_json::json!({
                "action": "dry-run",
                "mkdir": dest.display().to_string(),
                "copy": entry_strs,
            });
            let s = serde_json::to_string_pretty(&v).map_err(|e| e.to_string())?;
            writeln!(out, "{s}").map_err(|e| e.to_string())?;
        }
    }
    Ok(())
}

fn list_entries(source: &Path) -> Result<Vec<PathBuf>, String> {
    let mut entries: Vec<PathBuf> = fs::read_dir(source)
        .map_err(|e| format!("cannot read {}: {e}", source.display()))?
        .filter_map(|entry| {
            entry.ok().map(|e| {
                let path = e.path();
                if path
                    .file_name()
                    .map(|n| n != "." && n != "..")
                    .unwrap_or(false)
                {
                    Some(path.clone())
                } else {
                    None
                }
            })
        })
        .flatten()
        .collect();
    entries.sort();
    Ok(entries)
}

fn copy_skill_tree(source: &Path, dest: &Path) -> Result<(), String> {
    fs::create_dir_all(dest).map_err(|e| format!("cannot create {}: {e}", dest.display()))?;

    let entries = list_entries(source)?;
    for entry in &entries {
        let name = entry
            .file_name()
            .ok_or_else(|| format!("invalid entry: {}", entry.display()))?;
        let target = dest.join(name);
        if entry.is_dir() {
            copy_dir_recursive(entry, &target)?;
        } else {
            fs::copy(entry, &target)
                .map_err(|e| format!("copy {} -> {}: {e}", entry.display(), target.display()))?;
        }
    }
    Ok(())
}

fn copy_dir_recursive(src: &Path, dst: &Path) -> Result<(), String> {
    fs::create_dir_all(dst).map_err(|e| format!("cannot create {}: {e}", dst.display()))?;
    let entries = list_entries(src)?;
    for entry in &entries {
        let name = entry
            .file_name()
            .ok_or_else(|| format!("invalid entry: {}", entry.display()))?;
        let target = dst.join(name);
        if entry.is_dir() {
            copy_dir_recursive(entry, &target)?;
        } else {
            fs::copy(entry, &target)
                .map_err(|e| format!("copy {} -> {}: {e}", entry.display(), target.display()))?;
        }
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_install_defaults() {
        use clap::Parser;
        let opts = SkillInstallOptions::try_parse_from(["install"]).expect("parse defaults");
        assert!(!opts.force);
        assert!(!opts.dry_run);
        assert!(opts.dir.is_none());
        assert!(opts.source.is_none());
    }

    #[test]
    fn parse_install_flags() {
        use clap::Parser;
        let opts = SkillInstallOptions::try_parse_from([
            "install",
            "--dir",
            "/tmp/skills",
            "--force",
            "--dry-run",
        ])
        .expect("parse with flags");
        assert_eq!(opts.dir.as_deref(), Some("/tmp/skills"));
        assert!(opts.force);
        assert!(opts.dry_run);
    }
}
