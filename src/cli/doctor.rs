//! `mwt doctor` — inspect worktree registrations vs disk and suggest fixes.

use std::collections::{HashMap, HashSet};
use std::io::Write;

use clap::Args;

use crate::cli::deps::{Deps, SetupRunner};
use crate::cli::format::{OutputFormat, write_doctor};
use crate::cli::shared::{resolve_repo, run_serial};
use crate::config::Config;
use crate::doctor::{Checker, Finding, Kind, OsFs};

#[derive(Debug, Args)]
pub struct DoctorOptions {
    /// Re-run setup for all `setup_missing` findings.
    #[arg(long)]
    pub fix: bool,

    /// Best-effort: continue after a setup failure during `--fix`.
    #[arg(long)]
    pub r#continue: bool,

    /// Output format: `table` (default) or `json`.
    #[arg(long, value_enum, default_value_t = OutputFormat::Table)]
    pub format: OutputFormat,
}

pub fn run(
    deps: &Deps,
    opts: &DoctorOptions,
    setup_runner: &mut dyn SetupRunner,
    out: &mut dyn Write,
    err: &mut dyn Write,
) -> Result<(), String> {
    let cfg = (deps.load_config)().map_err(|e| format!("load config: {e}"))?;

    let git = GitListerFromDeps(&*deps.git);
    let osfs = OsFs;
    let checker = Checker::new(&git, &osfs);
    let findings = checker.check(&cfg).map_err(|e| format!("doctor: {e}"))?;

    hint_skill(err);

    write_doctor(out, opts.format, &findings).map_err(|e| e.to_string())?;

    if !opts.fix {
        return if findings.is_empty() {
            Ok(())
        } else {
            Err(format!("doctor found {} issue(s)", findings.len()))
        };
    }
    if !has_setup_missing(&findings) {
        return if findings.is_empty() {
            Ok(())
        } else {
            Err(format!("doctor found {} issue(s)", findings.len()))
        };
    }

    let fix_err = fix_setup_missing(&cfg, &findings, opts.r#continue, setup_runner, out)?;

    // Re-run after fix to report remaining state.
    let git2 = GitListerFromDeps(&*deps.git);
    let osfs2 = OsFs;
    let checker2 = Checker::new(&git2, &osfs2);
    let remaining = checker2.check(&cfg).map_err(|e| format!("doctor: {e}"))?;

    writeln!(out).map_err(|e| e.to_string())?;
    if let Some(fe) = fix_err {
        writeln!(out, "remaining after --fix:").map_err(|e| e.to_string())?;
        write_doctor(out, opts.format, &remaining).map_err(|e| e.to_string())?;
        return Err(format!("doctor --fix: {fe}"));
    }
    if remaining.is_empty() {
        writeln!(out, "fix: cleared setup_missing").map_err(|e| e.to_string())?;
        writeln!(out, "ok: no issues found").map_err(|e| e.to_string())?;
        return Ok(());
    }
    writeln!(out, "remaining after --fix:").map_err(|e| e.to_string())?;
    write_doctor(out, opts.format, &remaining).map_err(|e| e.to_string())?;
    Err(format!("doctor found {} issue(s)", remaining.len()))
}

fn has_setup_missing(findings: &[Finding]) -> bool {
    findings.iter().any(|f| f.kind == Kind::SetupMissing)
}

/// Emit `hint:` lines to stderr about the mwt Agent skill.
///
/// Diagnostics only: never fails, never affects the findings or exit code.
/// Checks (1) whether `~/.agents/skills/mwt/SKILL.md` exists and
/// (2) whether its frontmatter `version` matches the binary version.
fn hint_skill(err: &mut dyn Write) {
    // The skill install location is independent of the worktree meta-root,
    // so locate it under the user's home directory.
    let home = match std::env::var("HOME") {
        Ok(h) if !h.is_empty() => h,
        _ => return,
    };
    let skill_file = std::path::Path::new(&home).join(".agents/skills/mwt/SKILL.md");
    if !skill_file.is_file() {
        let _ = writeln!(
            err,
            "hint: mwt Agent skill not installed; run 'mwt skill install' or './scripts/install-skill.sh'"
        );
        return;
    }
    match read_skill_version(&skill_file) {
        Some(v) if v == crate::version::VERSION => {}
        Some(v) => {
            let _ = writeln!(
                err,
                "hint: skill version {v} differs from binary version {}; re-run 'mwt skill install' to update",
                crate::version::VERSION
            );
        }
        None => {}
    }
}

/// Parse the `version:` field out of a SKILL.md frontmatter block
/// (between the leading `---` and trailing `---`).
fn read_skill_version(path: &std::path::Path) -> Option<String> {
    let content = std::fs::read_to_string(path).ok()?;
    let mut in_frontmatter = false;
    for line in content.lines() {
        let trimmed = line.trim();
        if trimmed == "---" {
            if in_frontmatter {
                break;
            }
            in_frontmatter = true;
            continue;
        }
        if !in_frontmatter {
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("version:") {
            let v = rest.trim();
            // Allow both quoted and unquoted values.
            let v = v.trim_matches('"').trim_matches('\'');
            if !v.is_empty() {
                return Some(v.to_string());
            }
        }
    }
    None
}

/// `GitLister` adapter over `Deps::git`. Stores a `&dyn GitClient`
/// reference (not `&Deps`) so the surrounding `Deps` does not need `Sync`.
struct GitListerFromDeps<'a>(&'a dyn crate::cli::deps::GitClient);

impl crate::doctor::GitLister for GitListerFromDeps<'_> {
    fn list(
        &self,
        repo_path: &std::path::Path,
    ) -> Result<Vec<crate::git::Worktree>, crate::doctor::GitListError> {
        self.0
            .list(repo_path)
            .map_err(crate::doctor::GitListError::Other)
    }
}

fn fix_setup_missing(
    cfg: &Config,
    findings: &[Finding],
    continue_on: bool,
    setup_runner: &mut dyn SetupRunner,
    out: &mut dyn Write,
) -> Result<Option<String>, String> {
    let by_branch = group_setup_missing_by_branch(cfg, findings);
    if by_branch.is_empty() {
        return Ok(None);
    }

    let setup: &[crate::config::SetupStep] = &cfg.setup;
    let mut branches: Vec<&String> = by_branch.keys().collect();
    branches.sort();

    let mut errs: Vec<String> = Vec::new();
    for branch in branches {
        let repos = by_branch.get(branch).expect("present");
        writeln!(out, "\nfix: setup {branch} --repos {}", repos.join(","))
            .map_err(|e| e.to_string())?;
        let branch_owned = branch.clone();
        let err = run_serial(repos, continue_on, |repo| -> Result<(), String> {
            let ctx = resolve_repo(cfg, repo, &branch_owned)?;
            require_worktree_dir(&ctx.worktree_path)?;
            setup_runner
                .run(&ctx, setup)
                .map_err(|e| format!("setup: {e}"))?;
            writeln!(out, "{repo}\t{}", ctx.worktree_path.display()).map_err(|e| e.to_string())?;
            Ok(())
        });
        if let Err(e) = err {
            if !continue_on {
                return Err(e);
            }
            errs.push(e);
        }
    }
    if errs.is_empty() {
        Ok(None)
    } else {
        Ok(Some(errs.join("; ")))
    }
}

fn group_setup_missing_by_branch(
    cfg: &Config,
    findings: &[Finding],
) -> HashMap<String, Vec<String>> {
    let mut repo_order: HashMap<&str, usize> = HashMap::new();
    for (i, r) in cfg.repos.iter().enumerate() {
        repo_order.insert(r.as_str(), i);
    }

    let mut seen: HashMap<String, HashSet<String>> = HashMap::new();
    for f in findings {
        if f.kind != Kind::SetupMissing || f.branch.is_empty() || f.repo.is_empty() {
            continue;
        }
        seen.entry(f.branch.clone())
            .or_default()
            .insert(f.repo.clone());
    }

    let mut out: HashMap<String, Vec<String>> = HashMap::new();
    for (branch, repos) in seen {
        let mut list: Vec<String> = repos.into_iter().collect();
        list.sort_by(|a, b| {
            let oa = repo_order.get(a.as_str()).copied();
            let ob = repo_order.get(b.as_str()).copied();
            match (oa, ob) {
                (Some(i), Some(j)) => i.cmp(&j),
                (Some(_), None) => std::cmp::Ordering::Less,
                (None, Some(_)) => std::cmp::Ordering::Greater,
                (None, None) => a.cmp(b),
            }
        });
        out.insert(branch, list);
    }
    out
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
