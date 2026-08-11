//! Integration tests against a real system `git` binary.
//!
//! Skipped automatically when `git` is not on PATH.

use std::path::Path;
use std::process::Command;

use mwt::git::{Adapter, ExecRunner, Runner, parse_worktree_porcelain};

fn git_available() -> bool {
    Command::new("git").arg("--version").output().is_ok()
}

fn run(cwd: &Path, args: &[&str]) {
    let status = Command::new("git")
        .current_dir(cwd)
        .args(args)
        .status()
        .expect("git failed");
    assert!(status.success(), "git {args:?} failed in {}", cwd.display());
}

fn init_repo(dir: &Path) {
    run(dir, &["init", "-q", "-b", "main"]);
    run(dir, &["config", "user.email", "test@example.com"]);
    run(dir, &["config", "user.name", "Tester"]);
    run(dir, &["config", "commit.gpgsign", "false"]);
    std::fs::write(dir.join("README.md"), "init").unwrap();
    run(dir, &["add", "README.md"]);
    run(dir, &["commit", "-q", "-m", "init"]);
}

#[test]
fn add_existing_branch_creates_worktree() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    // Create a second branch we can checkout in the new worktree.
    run(repo, &["branch", "feat-x"]);
    let wt = repo.join("wt");

    let adapter = Adapter::new();
    adapter
        .add(repo, &wt, "feat-x", "")
        .expect("add should succeed when branch exists");

    // Worktree should be a working directory
    assert!(wt.join("README.md").is_file());

    let list = adapter.list(repo).unwrap();
    assert_eq!(list.len(), 2);
    assert_eq!(list[0].branch, "main");
    assert_eq!(list[1].branch, "feat-x");
}

#[test]
fn add_with_start_point_creates_branch() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    let wt = repo.join("wt-feat");

    let adapter = Adapter::new();
    adapter
        .add(repo, &wt, "feat-x", "main")
        .expect("add with -b <from> should succeed");

    assert!(wt.join("README.md").is_file());
    assert!(adapter.branch_exists(repo, "feat-x").unwrap());
}

#[test]
fn add_without_from_does_not_retry_when_branch_missing() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    let wt = repo.join("wt");

    let adapter = Adapter::new();
    // from is empty — we expect failure and **no** branch to be created.
    let err = adapter.add(repo, &wt, "feat-missing", "").unwrap_err();
    // Surface the original git error
    assert!(err.to_string().contains("git "));
    assert!(!adapter.branch_exists(repo, "feat-missing").unwrap());
}

#[test]
fn list_returns_empty_for_single_checkout() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    let adapter = Adapter::new();
    let list = adapter.list(repo).unwrap();
    assert_eq!(list.len(), 1);
    assert_eq!(list[0].branch, "main");
}

#[test]
fn remove_cleans_up() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    run(repo, &["branch", "feat-x"]);
    let wt = repo.join("wt");
    let adapter = Adapter::new();
    adapter.add(repo, &wt, "feat-x", "").unwrap();
    assert!(wt.exists());
    adapter.remove(repo, &wt, false).unwrap();
    assert!(!wt.exists());
}

#[test]
fn branch_exists_distinguishes_missing() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    let adapter = Adapter::new();
    assert!(adapter.branch_exists(repo, "main").unwrap());
    assert!(!adapter.branch_exists(repo, "nope").unwrap());
}

#[test]
fn porcelain_parser_is_consistent_with_git() {
    if !git_available() {
        eprintln!("git not on PATH, skipping");
        return;
    }
    let tmp = tempfile::tempdir().unwrap();
    let repo = tmp.path();
    init_repo(repo);
    run(repo, &["branch", "feat-x"]);
    let wt = repo.join("wt");
    let adapter = Adapter::new();
    adapter.add(repo, &wt, "feat-x", "").unwrap();

    let out = ExecRunner::new()
        .git(repo, ["worktree", "list", "--porcelain"])
        .unwrap();
    let parsed = parse_worktree_porcelain(&out.stdout);
    assert_eq!(parsed.len(), 2);
    let adapter_list = adapter.list(repo).unwrap();
    assert_eq!(parsed, adapter_list);
}
