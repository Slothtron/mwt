//! Walk `cfg.WorktreePath` with `{{BRANCH}}` as a wildcard for one repo.

use std::collections::HashMap;
use std::io;

use once_cell::sync::Lazy;
use regex::Regex;
use thiserror::Error;

use crate::doctor::Fs;
use crate::pathresolve::{
    PH_BRANCH, PH_MAIN_PATH, PH_REPO, PH_REPO_PATH, PH_ROOT, PH_WORKTREE_NAME, PH_WORKTREE_PATH,
};

const BRANCH_MARKER: &str = "{{BRANCH}}";

static TEMPLATE_RE: Lazy<Regex> = Lazy::new(|| Regex::new(r"\{\{([^{}]*)\}\}").unwrap());

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DiskWorktree {
    pub branch: String,
    pub path: String,
}

/// Walk `cfg.WorktreePath` (with `{{BRANCH}}` as a wildcard) on disk.
/// Returns disk-only candidates that are directories (and have a non-empty
/// `branch_so_far` chain).
///
/// Templates without `{{BRANCH}}` yield an empty list.
pub fn discover_disk_worktrees(
    fsys: &dyn Fs,
    meta_root: &std::path::Path,
    template: &str,
    repo: &str,
) -> Result<Vec<DiskWorktree>, ScanError> {
    if !template.contains(BRANCH_MARKER) {
        return Ok(Vec::new());
    }

    let main_path = meta_root.join(repo);
    let values: HashMap<&'static str, String> = [
        (PH_ROOT, meta_root.to_string_lossy().to_string()),
        (PH_REPO, repo.to_string()),
        (PH_REPO_PATH, repo.to_string()),
        (PH_MAIN_PATH, main_path.to_string_lossy().to_string()),
    ]
    .into_iter()
    .collect();

    let expanded = expand_template_literals(template, &values).map_err(ScanError::Template)?;

    let pattern = if std::path::Path::new(&expanded).is_absolute() {
        expanded
    } else {
        meta_root.join(&expanded).to_string_lossy().to_string()
    };
    let pattern = clean_path_str(&pattern);

    let mut out: Vec<DiskWorktree> = Vec::new();
    walk_branch_pattern(fsys, &pattern, "", &mut out).map_err(ScanError::Walk)?;
    Ok(out)
}

fn expand_template_literals(
    template: &str,
    values: &HashMap<&'static str, String>,
) -> Result<String, String> {
    let mut err: Option<String> = None;
    let out = TEMPLATE_RE.replace_all(template, |caps: &regex::Captures<'_>| {
        if err.is_some() {
            return caps[0].to_string();
        }
        let name = caps[1].trim();
        if name == PH_BRANCH {
            return caps[0].to_string();
        }
        if name == PH_WORKTREE_PATH || name == PH_WORKTREE_NAME {
            err = Some(format!(
                "placeholder {} is forbidden in worktree_path",
                &caps[0]
            ));
            return caps[0].to_string();
        }
        match values.get(name) {
            Some(v) => v.clone(),
            None => {
                err = Some(format!("unknown placeholder {} in worktree_path", &caps[0]));
                caps[0].to_string()
            }
        }
    });
    if let Some(e) = err {
        return Err(e);
    }
    Ok(out.into_owned())
}

fn walk_branch_pattern(
    fsys: &dyn Fs,
    pattern: &str,
    branch_so_far: &str,
    out: &mut Vec<DiskWorktree>,
) -> Result<(), io::Error> {
    let marker = BRANCH_MARKER;
    let idx = match pattern.find(marker) {
        Some(i) => i,
        None => {
            return match fsys.stat(std::path::Path::new(pattern)) {
                Ok(()) => {
                    if !branch_so_far.is_empty() {
                        out.push(DiskWorktree {
                            branch: branch_so_far.to_string(),
                            path: clean_path_str(pattern),
                        });
                    }
                    Ok(())
                }
                Err(e) if e.kind() == io::ErrorKind::NotFound => Ok(()),
                Err(e) => Err(e),
            };
        }
    };

    let prefix = &pattern[..idx];
    let suffix = &pattern[idx + marker.len()..];
    let parent_raw = prefix
        .trim_end_matches(std::path::MAIN_SEPARATOR)
        .trim_end_matches('/');
    let parent = if parent_raw.is_empty() {
        std::path::MAIN_SEPARATOR.to_string()
    } else {
        parent_raw.to_string()
    };
    let parent = clean_path_str(&parent);

    let entries = match fsys.read_dir(std::path::Path::new(&parent)) {
        Ok(es) => es,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(e) => return Err(e),
    };
    for e in entries {
        if !e.is_dir {
            continue;
        }
        let name = e.name;
        if name == "." || name == ".." {
            continue;
        }
        let next_raw = format!("{prefix}{name}{suffix}");
        let next = clean_path_str(&next_raw);
        // If branch_so_far is empty, this iteration's name IS the branch.
        // Otherwise, deeper segments keep the outer branch (matches Go).
        let br = if branch_so_far.is_empty() {
            name.clone()
        } else {
            branch_so_far.to_string()
        };
        walk_branch_pattern(fsys, &next, &br, out)?;
    }
    Ok(())
}

fn clean_path_str(p: &str) -> String {
    let mut out = String::new();
    let mut abs = std::path::Path::new(p).has_root();
    for c in std::path::Path::new(p).components() {
        match c {
            std::path::Component::ParentDir => {
                while out.ends_with('/') {
                    out.pop();
                }
                if let Some(pos) = out.rfind('/') {
                    out.truncate(pos);
                } else {
                    out.clear();
                }
            }
            std::path::Component::CurDir => {}
            std::path::Component::RootDir => {
                abs = true;
                out.push('/');
            }
            std::path::Component::Prefix(_) => {
                out.push_str(&c.as_os_str().to_string_lossy());
            }
            std::path::Component::Normal(s) => {
                if abs || !out.is_empty() {
                    out.push('/');
                }
                out.push_str(&s.to_string_lossy());
            }
        }
    }
    if abs && out.is_empty() {
        return "/".to_string();
    }
    if out.is_empty() {
        return ".".to_string();
    }
    out
}

#[derive(Debug, Error)]
pub enum ScanError {
    #[error("doctor: {0}")]
    Template(String),
    #[error("doctor: walk: {0}")]
    Walk(#[source] io::Error),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::doctor::{DirEntry, Fs};
    use std::collections::HashSet;
    use std::path::{Path, PathBuf};

    /// Minimal in-memory FS for the scan walker. Only `read_dir` and `stat`
    /// are implemented.
    #[derive(Default)]
    struct ScanFs {
        dirs: HashSet<PathBuf>,
        files: HashSet<PathBuf>,
    }

    impl ScanFs {
        fn add_dir(&mut self, p: &str) {
            self.dirs.insert(PathBuf::from(p));
        }
    }

    impl Fs for ScanFs {
        fn stat(&self, name: &Path) -> io::Result<()> {
            if self.dirs.contains(name) {
                Ok(())
            } else {
                Err(io::Error::new(io::ErrorKind::NotFound, "missing"))
            }
        }
        fn read_dir(&self, name: &Path) -> io::Result<Vec<DirEntry>> {
            if !self.dirs.contains(name) {
                return Err(io::Error::new(io::ErrorKind::NotFound, "missing"));
            }
            let mut out = Vec::new();
            for d in &self.dirs {
                if d.parent() == Some(name) {
                    out.push(DirEntry {
                        name: d.file_name().unwrap().to_string_lossy().to_string(),
                        is_dir: true,
                    });
                }
            }
            for f in &self.files {
                if f.parent() == Some(name) {
                    out.push(DirEntry {
                        name: f.file_name().unwrap().to_string_lossy().to_string(),
                        is_dir: false,
                    });
                }
            }
            Ok(out)
        }
    }

    #[test]
    fn template_without_branch_marker_returns_empty() {
        let fs = ScanFs::default();
        let out = discover_disk_worktrees(&fs, Path::new("/meta"), "wt/{{REPO}}", "api").unwrap();
        assert!(out.is_empty());
    }

    #[test]
    fn discovers_two_branches() {
        let mut fs = ScanFs::default();
        let template = "/meta/.worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}";
        // Walk the pattern with {{BRANCH}} as a wildcard, so each segment in
        // the path needs an entry for read_dir to surface it.
        for p in [
            "/meta",
            "/meta/.worktrees",
            "/meta/.worktrees/api",
            "/meta/.worktrees/api/feat-x",
            "/meta/.worktrees/api/feat-x/api",
            "/meta/.worktrees/api/feat-y",
            "/meta/.worktrees/api/feat-y/api",
        ] {
            fs.add_dir(p);
        }
        let out = discover_disk_worktrees(&fs, Path::new("/meta"), template, "api").unwrap();
        assert_eq!(out.len(), 2);
        let branches: Vec<_> = out.iter().map(|d| d.branch.clone()).collect();
        assert!(branches.contains(&"feat-x".to_string()));
        assert!(branches.contains(&"feat-y".to_string()));
    }

    #[test]
    fn unknown_placeholder_errors() {
        let fs = ScanFs::default();
        let err = discover_disk_worktrees(&fs, Path::new("/meta"), "{{NOPE}}/{{BRANCH}}", "api");
        assert!(err.is_err());
    }

    #[test]
    fn worktree_path_self_ref_forbidden() {
        let fs = ScanFs::default();
        let err = discover_disk_worktrees(
            &fs,
            Path::new("/meta"),
            "{{WORKTREE_PATH}}/{{BRANCH}}",
            "api",
        );
        assert!(err.is_err());
    }
}
