//! Walk up from a starting directory looking for `.mwt.yaml`.

use std::path::{Path, PathBuf};

use thiserror::Error;

pub const CONFIG_FILE_NAME: &str = ".mwt.yaml";

/// Walk upward from `start_dir` until `.mwt.yaml` is found.
///
/// Returns the absolute path of the first matching regular file. The walk
/// stops when `parent == dir` (filesystem root) and returns [`FindError::NotFound`].
///
/// Symlink loops are **not** defended against; the Go version also does not
/// defend against this.
pub fn find_config_file(start_dir: &Path) -> Result<PathBuf, FindError> {
    let mut dir = start_dir.canonicalize().map_err(|e| FindError::Stat {
        path: start_dir.to_path_buf(),
        source: e,
    })?;

    loop {
        let candidate = dir.join(CONFIG_FILE_NAME);
        let meta = std::fs::symlink_metadata(&candidate);
        match meta {
            Ok(m) if m.file_type().is_file() => return Ok(candidate),
            Ok(m) if m.file_type().is_dir() => {
                // exists but is a directory; treat as not-found and keep walking
            }
            Err(e) if e.kind() != std::io::ErrorKind::NotFound => {
                return Err(FindError::Stat { path: candidate, source: e });
            }
            _ => {}
        }

        let parent = dir.parent().map(|p| p.to_path_buf());
        match parent {
            Some(p) if p != dir => dir = p,
            _ => {
                return Err(FindError::NotFound(start_dir.to_path_buf()));
            }
        }
    }
}

#[derive(Debug, Error)]
pub enum FindError {
    #[error("no {CONFIG_FILE_NAME} found from {start} upward", start = .0.display())]
    NotFound(PathBuf),
    #[error("stat {path}: {source}")]
    Stat {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
}

#[cfg(test)]
mod tests {
    use super::*;

    fn touch(p: &Path) {
        std::fs::create_dir_all(p.parent().unwrap()).unwrap();
        std::fs::write(p, "").unwrap();
    }

    #[test]
    fn finds_in_current_dir() {
        let tmp = tempfile::tempdir().unwrap();
        touch(&tmp.path().join(CONFIG_FILE_NAME));
        let found = find_config_file(tmp.path()).unwrap();
        assert_eq!(found.file_name().unwrap(), CONFIG_FILE_NAME);
    }

    #[test]
    fn finds_in_parent() {
        let tmp = tempfile::tempdir().unwrap();
        touch(&tmp.path().join(CONFIG_FILE_NAME));
        let nested = tmp.path().join("a/b/c");
        std::fs::create_dir_all(&nested).unwrap();
        let found = find_config_file(&nested).unwrap();
        assert!(found.starts_with(tmp.path()));
    }

    #[test]
    fn not_found_at_root() {
        let tmp = tempfile::tempdir().unwrap();
        let result = find_config_file(tmp.path());
        assert!(matches!(result, Err(FindError::NotFound(_))));
    }

    #[test]
    fn directory_with_name_does_not_count() {
        // A directory named .mwt.yaml must not be returned.
        let tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(tmp.path().join(CONFIG_FILE_NAME)).unwrap();
        let result = find_config_file(tmp.path());
        assert!(matches!(result, Err(FindError::NotFound(_))));
    }
}
