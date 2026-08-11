//! 复制 / 运行 setup 步骤,作用于单个 worktree 的 [`pathresolve::Context`]。
//!
//! 与 Go 版 `mwt-legacy/internal/setup/runner.go` 行为一致。

use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};

use thiserror::Error;

use crate::config::{CopyAction, RunAction, SetupStep};
use crate::pathresolve::{Context, Stage};

#[derive(Debug, Error)]
pub enum SetupError {
    #[error("setup: ROOT is empty")]
    EmptyRoot,
    #[error("setup: WORKTREE_PATH is empty")]
    EmptyWorktreePath,
    #[error("setup: step {index}: {source}")]
    Step {
        index: usize,
        #[source]
        source: Box<SetupError>,
    },
    #[error("step must have exactly one action")]
    NoOrMultipleActions,
    #[error("copy.from: {0}")]
    CopyFrom(#[source] crate::pathresolve::ExpandError),
    #[error("copy.to: {0}")]
    CopyTo(#[source] crate::pathresolve::ExpandError),
    #[error("copy: stat destination {path}: {source}", path = .path.display())]
    StatDest {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("copy: source {path}: {source}", path = .path.display())]
    StatSrc {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("copy: source {0:?} is a directory")]
    SourceIsDir(PathBuf),
    #[error("copy {from} -> {to}: {source}", from = .from.display(), to = .to.display())]
    Copy {
        from: PathBuf,
        to: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("run.command: {0}")]
    RunCommand(#[source] crate::pathresolve::ExpandError),
    #[error("run.command is empty after expansion")]
    RunCommandEmpty,
    #[error("run.dir: {0}")]
    RunDir(#[source] crate::pathresolve::ExpandError),
    #[error("run {command:?} (cwd {cwd}): {source}", cwd = .cwd.display())]
    RunExec {
        command: String,
        cwd: PathBuf,
        #[source]
        source: std::io::Error,
    },
}

pub type StdWriter = Box<dyn Write + Send + Sync>;

/// Function that builds a `Command` for one invocation. Tests may inject a stub.
pub type CommandFactory = Box<dyn Fn(&str, &[&str]) -> Command + Send + Sync>;

pub struct Runner {
    pub stdout: Option<Stdio>,
    pub stderr: Option<Stdio>,
    /// If `None`, defaults to `Command::new(name).args(...)`.
    pub command: Option<CommandFactory>,
}

impl std::fmt::Debug for Runner {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Runner")
            .field("stdout", &self.stdout.is_some())
            .field("stderr", &self.stderr.is_some())
            .field("command", &self.command.is_some())
            .finish()
    }
}

impl Default for Runner {
    fn default() -> Self {
        Self::new()
    }
}

impl Runner {
    pub fn new() -> Self {
        Self { stdout: None, stderr: None, command: None }
    }

    pub fn with_stdout(mut self, stdio: Stdio) -> Self {
        self.stdout = Some(stdio);
        self
    }
    pub fn with_stderr(mut self, stdio: Stdio) -> Self {
        self.stderr = Some(stdio);
        self
    }

    /// Run all steps in order against the given context.
    pub fn run(&mut self, ctx: &Context, steps: &[SetupStep]) -> Result<(), SetupError> {
        if ctx.root.as_os_str().is_empty() {
            return Err(SetupError::EmptyRoot);
        }
        if ctx.worktree_path.as_os_str().is_empty() {
            return Err(SetupError::EmptyWorktreePath);
        }

        for (i, step) in steps.iter().enumerate() {
            self.run_step(ctx, step).map_err(|e| SetupError::Step {
                index: i,
                source: Box::new(e),
            })?;
        }
        Ok(())
    }

    fn run_step(&mut self, ctx: &Context, step: &SetupStep) -> Result<(), SetupError> {
        match (&step.copy, &step.run) {
            (Some(_), Some(_)) | (None, None) => Err(SetupError::NoOrMultipleActions),
            (Some(c), None) => self.copy(ctx, c),
            (None, Some(r)) => self.run_action(ctx, r),
        }
    }

    fn copy(&self, ctx: &Context, action: &CopyAction) -> Result<(), SetupError> {
        use std::os::unix::fs::PermissionsExt;
        let from_raw = ctx.expand(&action.from, Stage::Setup).map_err(SetupError::CopyFrom)?;
        let to_raw = ctx.expand(&action.to, Stage::Setup).map_err(SetupError::CopyTo)?;

        let from = abs_or_join(&from_raw, &ctx.root);
        let to = abs_or_join(&to_raw, &ctx.worktree_path);

        if action.skip_if_exists_or_default() {
            match fs::symlink_metadata(&to) {
                Ok(_) => return Ok(()),
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => {
                    return Err(SetupError::StatDest { path: to, source: e });
                }
            }
        }

        let src_info = match fs::symlink_metadata(&from) {
            Ok(m) => m,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound
                && action.skip_if_missing_src_or_default() =>
            {
                return Ok(());
            }
            Err(e) => {
                return Err(SetupError::StatSrc { path: from, source: e });
            }
        };

        if src_info.is_dir() {
            return Err(SetupError::SourceIsDir(from));
        }

        self.copy_file(&from, &to, src_info.permissions().mode())?;
        Ok(())
    }

    fn run_action(&mut self, ctx: &Context, action: &RunAction) -> Result<(), SetupError> {
        let command = ctx
            .expand(&action.command, Stage::Setup)
            .map_err(SetupError::RunCommand)?;
        if command.is_empty() {
            return Err(SetupError::RunCommandEmpty);
        }

        let dir = match &action.dir {
            Some(d) => {
                let raw = ctx.expand(d, Stage::Setup).map_err(SetupError::RunDir)?;
                abs_or_join(&raw, &ctx.worktree_path)
            }
            None => ctx.worktree_path.clone(),
        };

        let mut cmd = self.spawn_command("sh", &["-c", command.as_str()]);
        cmd.current_dir(&dir);
        // Move out of `Option<Stdio>`; we may clear it but `Runner` is per-call.
        let stdout = self.stdout.take();
        let stderr = self.stderr.take();
        if let Some(s) = stdout {
            cmd.stdout(s);
        }
        if let Some(s) = stderr {
            cmd.stderr(s);
        }
        cmd.status().map_err(|e| SetupError::RunExec {
            command,
            cwd: dir,
            source: e,
        })?;
        Ok(())
    }

    fn spawn_command(&self, name: &str, args: &[&str]) -> Command {
        if let Some(f) = &self.command {
            f(name, args)
        } else {
            let mut c = Command::new(name);
            c.args(args);
            c
        }
    }

    fn copy_file(&self, from: &Path, to: &Path, perm: u32) -> Result<(), SetupError> {
        use std::os::unix::fs::PermissionsExt;
        let data = fs::read(from).map_err(|e| SetupError::Copy {
            from: from.to_path_buf(),
            to: to.to_path_buf(),
            source: e,
        })?;
        if let Some(parent) = to.parent() {
            fs::create_dir_all(parent).map_err(|e| SetupError::Copy {
                from: from.to_path_buf(),
                to: to.to_path_buf(),
                source: e,
            })?;
        }
        {
            let mut f = fs::OpenOptions::new()
                .create(true)
                .write(true)
                .truncate(true)
                .open(to)
                .map_err(|e| SetupError::Copy {
                    from: from.to_path_buf(),
                    to: to.to_path_buf(),
                    source: e,
                })?;
            f.write_all(&data).map_err(|e| SetupError::Copy {
                from: from.to_path_buf(),
                to: to.to_path_buf(),
                source: e,
            })?;
            f.sync_all().map_err(|e| SetupError::Copy {
                from: from.to_path_buf(),
                to: to.to_path_buf(),
                source: e,
            })?;
        }
        let _ = std::fs::set_permissions(to, std::fs::Permissions::from_mode(perm));
        Ok(())
    }
}

/// Absolute path or joined to `base`. Mirrors Go's `absOrJoin`.
fn abs_or_join(path: &str, base: &Path) -> PathBuf {
    let p = Path::new(path);
    if p.is_absolute() {
        clean(p)
    } else {
        clean(&base.join(p))
    }
}

fn clean(p: &Path) -> PathBuf {
    let mut out = PathBuf::new();
    for c in p.components() {
        match c {
            std::path::Component::ParentDir => {
                out.pop();
            }
            std::path::Component::CurDir => {}
            other => out.push(other.as_os_str()),
        }
    }
    if out.as_os_str().is_empty() {
        PathBuf::from(".")
    } else {
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{CopyAction, RunAction, SetupStep};
    use crate::pathresolve::Context;
    use std::path::PathBuf;

    fn ctx(worktree: &str) -> Context {
        let root = PathBuf::from("/tmp/setup-test-meta");
        Context {
            root: root.clone(),
            repo: "api".into(),
            repo_path: "api".into(),
            main_path: root.join("api"),
            branch: "feat-x".into(),
            worktree_path: PathBuf::from(worktree),
            worktree_name: Path::new(worktree).file_name().unwrap().to_string_lossy().to_string(),
        }
    }

    #[test]
    fn empty_steps_is_noop() {
        let mut r = Runner::new();
        r.run(&ctx("/tmp/wt"), &[]).unwrap();
    }

    #[test]
    fn rejects_empty_root() {
        let mut r = Runner::new();
        let mut c = ctx("/tmp/wt");
        c.root = PathBuf::new();
        let err = r.run(&c, &[]).unwrap_err();
        assert!(matches!(err, SetupError::EmptyRoot));
    }

    #[test]
    fn rejects_no_action_step() {
        let mut r = Runner::new();
        let step = SetupStep { copy: None, run: None };
        let err = r.run(&ctx("/tmp/wt"), &[step]).unwrap_err();
        assert!(matches!(err, SetupError::Step { .. }));
    }
}
