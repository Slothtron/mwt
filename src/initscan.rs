//! 递归发现 `.git` 目录(用于 `mwt init`)。

use std::collections::HashSet;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use thiserror::Error;

const SKIP_DIR_NAMES: &[&str] = &[".git", "worktrees", ".worktrees"];

/// Walk `root` downward up to `max_depth` levels looking for Git checkouts.
/// Returns slash-normalized relative paths (YAML-portable), sorted.
///
/// `max_depth == 0` checks only `root` itself. A checkout at `root` is
/// returned as `"."`. When a git root is found, its children are not
/// descended into. Names in `SKIP_DIR_NAMES` are neither entered nor listed.
pub fn discover(root: &Path, max_depth: usize) -> Result<Vec<String>, DiscoverError> {
    if max_depth > i32::MAX as usize {
        return Err(DiscoverError::BadDepth(max_depth));
    }
    let abs_root = std::path::absolute(root).map_err(|e| DiscoverError::ResolveRoot {
        path: root.to_path_buf(),
        source: e,
    })?;
    let meta = fs::metadata(&abs_root).map_err(|e| DiscoverError::Stat {
        path: abs_root.clone(),
        source: e,
    })?;
    if !meta.is_dir() {
        return Err(DiscoverError::NotADirectory(abs_root));
    }

    let mut repos: Vec<String> = Vec::new();
    walk(&abs_root, &abs_root, 0, max_depth, &mut repos)?;
    repos.sort();
    Ok(repos)
}

fn walk(
    abs_root: &Path,
    dir: &Path,
    depth: usize,
    max_depth: usize,
    repos: &mut Vec<String>,
) -> Result<(), DiscoverError> {
    if has_git(dir) {
        let rel = rel_repo(abs_root, dir)?;
        repos.push(rel);
        return Ok(()); // do not descend into a git checkout
    }
    if depth >= max_depth {
        return Ok(());
    }

    let entries = fs::read_dir(dir).map_err(|e| DiscoverError::ReadDir {
        path: dir.to_path_buf(),
        source: e,
    })?;
    let skip: HashSet<&str> = SKIP_DIR_NAMES.iter().copied().collect();

    for entry in entries {
        let entry = entry.map_err(|e| DiscoverError::ReadDir {
            path: dir.to_path_buf(),
            source: e,
        })?;
        let ft = match entry.file_type() {
            Ok(t) => t,
            Err(_) => continue,
        };
        if !ft.is_dir() {
            continue;
        }
        let name = entry.file_name();
        let name = name.to_string_lossy();
        if skip.contains(name.as_ref()) {
            continue;
        }
        let child = dir.join(&*name);
        walk(abs_root, &child, depth + 1, max_depth, repos)?;
    }
    Ok(())
}

fn has_git(dir: &Path) -> bool {
    // `symlink_metadata` to follow gitfile indirection (matches Go's `os.Stat`).
    fs::symlink_metadata(dir.join(".git")).is_ok()
}

fn rel_repo(abs_root: &Path, dir: &Path) -> Result<String, DiscoverError> {
    let rel = dir.strip_prefix(abs_root).map_err(|e| DiscoverError::Rel {
        from: abs_root.to_path_buf(),
        to: dir.to_path_buf(),
        source: io::Error::other(e),
    })?;
    if rel.as_os_str().is_empty() {
        return Ok(".".to_string());
    }
    Ok(path_to_slash(rel))
}

/// Convert a path to use forward slashes (YAML portability).
fn path_to_slash(p: &Path) -> String {
    let s = p.to_string_lossy();
    s.replace(std::path::MAIN_SEPARATOR, "/")
}

#[derive(Debug, Error)]
pub enum DiscoverError {
    #[error("initscan: max depth must fit in i32: {0}")]
    BadDepth(usize),
    #[error("initscan: resolve root {path}: {source}", path = .path.display())]
    ResolveRoot {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("initscan: root: {path}", path = .path.display())]
    Stat {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("initscan: root is not a directory: {0:?}")]
    NotADirectory(PathBuf),
    #[error("initscan: read {path}: {source}", path = .path.display())]
    ReadDir {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("initscan: relative path from {from} to {to}: {source}", from = .from.display(), to = .to.display())]
    Rel {
        from: PathBuf,
        to: PathBuf,
        #[source]
        source: io::Error,
    },
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn mkdir(p: &Path) {
        fs::create_dir_all(p).unwrap();
    }

    fn make_git(p: &Path) {
        mkdir(&p.join(".git"));
    }

    #[test]
    fn depth_zero_only_root() {
        let tmp = tempfile::tempdir().unwrap();
        let repos = discover(tmp.path(), 0).unwrap();
        assert_eq!(repos, Vec::<String>::new());

        make_git(tmp.path());
        let repos = discover(tmp.path(), 0).unwrap();
        assert_eq!(repos, vec!["."]);
    }

    #[test]
    fn discovers_nested_checkouts() {
        let tmp = tempfile::tempdir().unwrap();
        make_git(&tmp.path().join("a"));
        mkdir(&tmp.path().join("b/c"));
        make_git(&tmp.path().join("b/c"));
        let mut repos = discover(tmp.path(), 5).unwrap();
        repos.sort();
        assert_eq!(repos, vec!["a", "b/c"]);
    }

    #[test]
    fn does_not_descend_into_git_checkout() {
        let tmp = tempfile::tempdir().unwrap();
        let a = tmp.path().join("a");
        make_git(&a);
        // nested .git inside a/ should NOT be discovered as a separate repo
        mkdir(&a.join(".git")); // already a dir from make_git
        let repos = discover(tmp.path(), 10).unwrap();
        assert_eq!(repos, vec!["a"]);
    }

    #[test]
    fn skips_worktrees_and_dotgit_dirs() {
        let tmp = tempfile::tempdir().unwrap();
        // a/worktrees/x/.git should not be discovered (worktrees skipped)
        mkdir(&tmp.path().join("a/worktrees/x"));
        make_git(&tmp.path().join("a/worktrees/x"));
        // a/.worktrees/x/.git should not be discovered
        mkdir(&tmp.path().join("a/.worktrees/x"));
        make_git(&tmp.path().join("a/.worktrees/x"));
        // a/.git must not be discovered as a repo itself
        make_git(&tmp.path().join("a"));
        let repos = discover(tmp.path(), 10).unwrap();
        assert_eq!(repos, vec!["a"]);
    }

    #[test]
    fn normalizes_separators() {
        let tmp = tempfile::tempdir().unwrap();
        make_git(&tmp.path().join("a/b"));
        let repos = discover(tmp.path(), 5).unwrap();
        assert_eq!(repos, vec!["a/b"]);
        // The relative portion must never contain a backslash (Windows-only
        // separator that would not be portable into YAML / git porcelain).
        assert!(!repos[0].contains('\\'));
    }

    #[test]
    fn rejects_non_directory_root() {
        let tmp = tempfile::tempdir().unwrap();
        let f = tmp.path().join("file");
        fs::write(&f, "").unwrap();
        assert!(matches!(
            discover(&f, 1).unwrap_err(),
            DiscoverError::NotADirectory(_)
        ));
    }
}
