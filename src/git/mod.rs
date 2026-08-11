//! git worktree operations — shells out to system `git` via `std::process::Command`.
//!
//! 与 Go 版 `mwt-legacy/internal/git/` 行为一致。

use std::ffi::OsStr;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};

use thiserror::Error;

pub mod porcelain;

pub use porcelain::{parse_worktree_porcelain, Worktree};

/// Executes one git command: `git -C <repo_path> <args...>`.
pub trait Runner: Send + Sync {
    fn git<I, S>(&self, repo_path: &Path, args: I) -> Result<GitOutput, GitError>
    where
        I: IntoIterator<Item = S>,
        S: AsRef<OsStr>;
}

/// Captured stdout / stderr of a successful git invocation.
#[derive(Debug, Clone)]
pub struct GitOutput {
    pub stdout: String,
}

impl GitOutput {
    pub fn empty() -> Self {
        Self { stdout: String::new() }
    }
}

/// Default `Runner` that invokes the system `git` binary.
#[derive(Debug, Default, Clone)]
pub struct ExecRunner {
    /// Binary name; defaults to `"git"`.
    pub bin: String,
}

impl ExecRunner {
    pub fn new() -> Self {
        Self { bin: "git".to_string() }
    }

    pub fn with_bin(bin: impl Into<String>) -> Self {
        Self { bin: bin.into() }
    }
}

impl Runner for ExecRunner {
    fn git<I, S>(&self, repo_path: &Path, args: I) -> Result<GitOutput, GitError>
    where
        I: IntoIterator<Item = S>,
        S: AsRef<OsStr>,
    {
        let bin = if self.bin.is_empty() { "git" } else { &self.bin };
        let collected: Vec<String> = args
            .into_iter()
            .map(|s| s.as_ref().to_string_lossy().to_string())
            .collect();
        let output = Command::new(bin)
            .arg("-C")
            .arg(repo_path.as_os_str())
            .args(&collected)
            .output()
            .map_err(|e| GitError::Spawn {
                repo_path: repo_path.to_path_buf(),
                args: collected.clone(),
                source: e,
            })?;

        let stdout = String::from_utf8_lossy(&output.stdout).into_owned();
        let stderr = String::from_utf8_lossy(&output.stderr).into_owned().trim().to_string();

        if output.status.success() {
            Ok(GitOutput { stdout })
        } else {
            let code = output.status.code();
            Err(GitError::Command {
                repo_path: repo_path.to_path_buf(),
                args: collected,
                stderr,
                code,
            })
        }
    }
}

/// Failure categories for git invocations.
#[derive(Debug, Error)]
pub enum GitError {
    #[error("git {args} (repo {repo_path}): spawn failed: {source}", args = .args.join(" "), repo_path = .repo_path.display())]
    Spawn {
        repo_path: PathBuf,
        args: Vec<String>,
        #[source]
        source: std::io::Error,
    },
    #[error("git {args} (repo {repo_path}): {stderr}", args = .args.join(" "), repo_path = .repo_path.display())]
    Command {
        repo_path: PathBuf,
        args: Vec<String>,
        stderr: String,
        code: Option<i32>,
    },
}

impl GitError {
    pub fn repo_path(&self) -> &Path {
        match self {
            GitError::Spawn { repo_path, .. } | GitError::Command { repo_path, .. } => repo_path,
        }
    }
    pub fn args(&self) -> &[String] {
        match self {
            GitError::Spawn { args, .. } | GitError::Command { args, .. } => args,
        }
    }
    pub fn stderr(&self) -> &str {
        match self {
            GitError::Spawn { .. } => "",
            GitError::Command { stderr, .. } => stderr,
        }
    }
    /// `true` if this is a non-zero exit and the process exit code is `want`.
    pub fn exit_code_is(&self, want: i32) -> bool {
        matches!(self, GitError::Command { code: Some(c), .. } if *c == want)
    }
}

/// Public API used by the CLI; mirrors Go's `git.Adapter`.
pub struct Adapter<R: Runner> {
    runner: R,
}

impl Adapter<ExecRunner> {
    pub fn new() -> Self {
        Self { runner: ExecRunner::new() }
    }
}

impl<R: Runner> Adapter<R> {
    pub fn with_runner(runner: R) -> Self {
        Self { runner }
    }

    pub fn runner(&self) -> &R {
        &self.runner
    }

    /// `git worktree add <path> <branch>`, retrying with `-b <branch> <path> <from>`
    /// if the branch is missing and a start point is supplied.
    pub fn add(
        &self,
        repo_path: &Path,
        worktree_path: &Path,
        branch: &str,
        from: &str,
    ) -> Result<(), GitError> {
        if repo_path.as_os_str().is_empty() {
            return Err(GitError::Command {
                repo_path: PathBuf::new(),
                args: vec!["worktree".into(), "add".into()],
                stderr: "repo path is empty".into(),
                code: None,
            });
        }
        if worktree_path.as_os_str().is_empty() {
            return Err(GitError::Command {
                repo_path: repo_path.to_path_buf(),
                args: vec!["worktree".into(), "add".into()],
                stderr: format!("worktree path is empty (repo {})", repo_path.display()),
                code: None,
            });
        }
        if branch.is_empty() {
            return Err(GitError::Command {
                repo_path: repo_path.to_path_buf(),
                args: vec!["worktree".into(), "add".into()],
                stderr: format!("branch is empty (repo {})", repo_path.display()),
                code: None,
            });
        }

        let primary = self.runner.git(
            repo_path,
            ["worktree", "add", worktree_path.to_str().unwrap_or("."), branch],
        );
        if primary.is_ok() {
            return Ok(());
        }

        // No retry without a start point.
        if from.is_empty() {
            return primary.map(|_| ()).map_err(|e| e);
        }

        // Retry only if the branch does not yet exist.
        match self.branch_exists(repo_path, branch) {
            Ok(true) => {
                // Branch exists; surface the original failure (path busy, etc.).
                primary.map(|_| ())
            }
            Ok(false) => self
                .runner
                .git(
                    repo_path,
                    [
                        "worktree",
                        "add",
                        "-b",
                        branch,
                        worktree_path.to_str().unwrap_or("."),
                        from,
                    ],
                )
                .map(|_| ()),
            Err(branch_err) => Err(GitError::Command {
                repo_path: repo_path.to_path_buf(),
                args: vec!["show-ref".into(), "--verify".into(), "--quiet".into()],
                stderr: format!(
                    "{} (also failed to check branch: {})",
                    primary.as_ref().err().map(|e| e.to_string()).unwrap_or_default(),
                    branch_err
                ),
                code: None,
            }),
        }
    }

    /// `git worktree remove [--force] <path>`.
    pub fn remove(
        &self,
        repo_path: &Path,
        worktree_path: &Path,
        force: bool,
    ) -> Result<(), GitError> {
        if repo_path.as_os_str().is_empty() {
            return Err(GitError::Command {
                repo_path: PathBuf::new(),
                args: vec!["worktree".into(), "remove".into()],
                stderr: "repo path is empty".into(),
                code: None,
            });
        }
        if worktree_path.as_os_str().is_empty() {
            return Err(GitError::Command {
                repo_path: repo_path.to_path_buf(),
                args: vec!["worktree".into(), "remove".into()],
                stderr: format!("worktree path is empty (repo {})", repo_path.display()),
                code: None,
            });
        }
        let mut args: Vec<String> = vec!["worktree".into(), "remove".into()];
        if force {
            args.push("--force".into());
        }
        args.push(worktree_path.to_string_lossy().to_string());
        self.runner
            .git(repo_path, &args)
            .map(|_| ())
    }

    /// `git worktree list --porcelain`.
    pub fn list(&self, repo_path: &Path) -> Result<Vec<Worktree>, GitError> {
        if repo_path.as_os_str().is_empty() {
            return Err(GitError::Command {
                repo_path: PathBuf::new(),
                args: vec!["worktree".into(), "list".into(), "--porcelain".into()],
                stderr: "repo path is empty".into(),
                code: None,
            });
        }
        let out = self.runner.git(repo_path, ["worktree", "list", "--porcelain"])?;
        Ok(parse_worktree_porcelain(&out.stdout))
    }

    /// `git show-ref --verify --quiet refs/heads/<branch>`. Treats exit code 1
    /// as "branch does not exist" (not an error).
    pub fn branch_exists(&self, repo_path: &Path, branch: &str) -> Result<bool, GitError> {
        if repo_path.as_os_str().is_empty() {
            return Err(GitError::Command {
                repo_path: PathBuf::new(),
                args: vec!["show-ref".into()],
                stderr: "repo path is empty".into(),
                code: None,
            });
        }
        if branch.is_empty() {
            return Err(GitError::Command {
                repo_path: repo_path.to_path_buf(),
                args: vec!["show-ref".into()],
                stderr: format!("branch is empty (repo {})", repo_path.display()),
                code: None,
            });
        }
        let ref_ = format!("refs/heads/{branch}");
        match self.runner.git(repo_path, ["show-ref", "--verify", "--quiet", &ref_]) {
            Ok(_) => Ok(true),
            Err(e) if e.exit_code_is(1) => Ok(false),
            Err(e) => Err(e),
        }
    }
}

// Convenience: `Output` adapter used by fake runners in tests.
pub fn capture_output(bin: &str, cwd: &Path, args: &[&str]) -> Result<Output, std::io::Error> {
    Command::new(bin).current_dir(cwd).args(args).output()
}
