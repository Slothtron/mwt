//! CLI 调度层(clap derive + 命令分发)。
//!
//! 公开入口 [`run`]:接收 `Cli` 与 `Deps`,返回 `ExitCode`。
//! `Deps` 持有所有可注入的协作者,`Cli` 由 clap 派生,所有子命令各自
//! 维护自己的 `Args` 结构体。

use std::io;
use std::process::ExitCode;

use clap::{Parser, Subcommand};

pub mod add;
pub mod deps;
pub mod doctor;
pub mod format;
pub mod init;
pub mod list;
pub mod meta_root;
pub mod path;
pub mod rm;
pub mod setup;
pub mod shared;
pub mod skill;
pub mod version;

use deps::{Deps, SetupRunner};
use setup as setup_cmd;

/// Top-level CLI parsed by clap.
#[derive(Debug, Parser)]
#[command(
    name = "mwt",
    version = env!("CARGO_PKG_VERSION"),
    about = "Multi-repo WorkTrees: polyrepo git worktree manager",
    long_about = "mwt (Multi-repo WorkTrees) manages git worktrees across multiple\n\
                  independent repositories from a single .mwt.yaml at the meta-root.",
    disable_help_subcommand = true,
)]
pub struct Cli {
    #[command(subcommand)]
    pub command: Command,
}

#[derive(Debug, Subcommand)]
pub enum Command {
    /// Scan for git checkouts and write `.mwt.yaml` in the current directory.
    Init(init::InitOptions),
    /// Print mwt version information.
    Version,
    /// Create worktrees for the branch across configured repos.
    Add {
        /// Branch name to create.
        branch: String,
        #[command(flatten)]
        options: add::AddOptions,
    },
    /// Remove worktrees for the branch across configured repos.
    Rm {
        branch: String,
        #[command(flatten)]
        options: rm::RmOptions,
    },
    /// List worktrees across configured repos.
    List(list::ListOptions),
    /// Print the absolute worktree path for branch and repo.
    Path {
        branch: String,
        repo: String,
        #[command(flatten)]
        options: path::PathOptions,
    },
    /// Run configured setup steps on existing worktrees.
    Setup {
        branch: String,
        #[command(flatten)]
        options: setup::SetupOptions,
    },
    /// Inspect worktree registrations vs disk and suggest fixes.
    Doctor(doctor::DoctorOptions),
    /// Install the mwt Agent skill to a user skills directory.
    #[command(subcommand)]
    Skill(skill::SkillCommand),
    /// Hidden helper: print the meta-root located by walking up for `.mwt.yaml`.
    #[command(name = "meta-root", hide = true)]
    MetaRoot,
}

/// Dispatch a parsed [`Cli`].
///
/// `stdout` / `stderr` / `setup` are passed in as independent borrows so
/// the subcommand bodies can read `&Deps` (git / load_config) at the same
/// time. The `Deps` itself is constructed by [`Deps::real`] inside
/// [`crate::run`].
pub fn run(
    cli: Cli,
    stdout: &mut dyn io::Write,
    stderr: &mut dyn io::Write,
    setup: &mut dyn SetupRunner,
    mut deps: Deps,
) -> ExitCode {
    match dispatch(&cli, stdout, stderr, setup, &mut deps) {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            let _ = writeln!(stderr, "error: {e}");
            ExitCode::FAILURE
        }
    }
}

fn dispatch(
    cli: &Cli,
    stdout: &mut dyn io::Write,
    stderr: &mut dyn io::Write,
    setup: &mut dyn SetupRunner,
    deps: &mut Deps,
) -> Result<(), String> {
    match &cli.command {
        Command::Init(opts) => init::run(deps, opts, stdout),
        Command::Version => version::run(stdout),
        Command::Add { branch, options } => add::run(deps, branch, options, setup, stdout),
        Command::Rm { branch, options } => rm::run(deps, branch, options, stdout),
        Command::List(opts) => list::run(deps, opts, stdout),
        Command::Path {
            branch,
            repo,
            options,
        } => path::run(deps, branch, repo, options, stdout),
        Command::Setup { branch, options } => setup_cmd::run(deps, branch, options, setup, stdout),
        Command::Doctor(opts) => doctor::run(deps, opts, setup, stdout, stderr),
        Command::Skill(cmd) => match cmd {
            skill::SkillCommand::Install(opts) => skill::run(opts, stdout),
        },
        Command::MetaRoot => meta_root::run(deps, stdout),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cli_parses_init_with_depth() {
        let cli = Cli::try_parse_from(["mwt", "init", "--depth", "3"]).unwrap();
        match cli.command {
            Command::Init(opts) => assert_eq!(opts.depth, 3),
            _ => panic!("expected init"),
        }
    }

    #[test]
    fn cli_parses_add_with_from_and_no_setup() {
        let cli = Cli::try_parse_from([
            "mwt",
            "add",
            "feat-x",
            "--from",
            "main",
            "--no-setup",
            "--continue",
        ])
        .unwrap();
        match cli.command {
            Command::Add { branch, options } => {
                assert_eq!(branch, "feat-x");
                assert_eq!(options.from, "main");
                assert!(options.no_setup);
                assert!(options.r#continue);
            }
            _ => panic!("expected add"),
        }
    }

    #[test]
    fn cli_rejects_unknown_format() {
        let cli = Cli::try_parse_from(["mwt", "list", "--format", "yaml"]);
        assert!(cli.is_err());
    }

    #[test]
    fn cli_accepts_json_format() {
        let cli = Cli::try_parse_from(["mwt", "list", "--format", "json"]).unwrap();
        match cli.command {
            Command::List(opts) => assert_eq!(opts.format, format::OutputFormat::Json),
            _ => panic!("expected list"),
        }
    }

    #[test]
    fn skill_subcommand_parses_install() {
        // Stage H: `mwt skill install` is the in-binary install path;
        // bare `mwt skill` still fails (requires a subcommand).
        let cli = Cli::try_parse_from(["mwt", "skill", "install"]);
        assert!(cli.is_ok(), "skill install should parse: {cli:?}");
        let cli = Cli::try_parse_from(["mwt", "skill"]);
        assert!(cli.is_err(), "bare skill should not parse");
    }
}
