---
name: mwt
version: "0.3.0"
description: >-
  Manage polyrepo Git worktrees with the mwt CLI (.mwt.yaml, add/rm/list/path/setup/doctor/init/skill install).
  Use when the user mentions mwt, .mwt.yaml, multi-repo worktrees, or polyrepo branch checkout paths.
---

# mwt — Multi-repo WorkTrees

CLI that creates / removes / lists / doctors Git worktrees across multiple independent repos from one `.mwt.yaml`.

## Install skill (user machine)

```bash
mwt skill install            # → ~/.agents/skills/mwt
mwt skill install --force    # overwrite
mwt skill install --dir PATH # → PATH/mwt
mwt skill install --dry-run  # print plan only
```

等价于仓库脚本 `./scripts/install-skill.sh`（二者行为一致）。`mwt doctor` 会在
未安装 skill 或版本不符时于 stderr 打印一行 `hint:`。

## Commands

| Command | Purpose |
|---------|---------|
| `mwt init` | Scan cwd for Git checkouts; write `.mwt.yaml` with `worktree_path` |
| `mwt add <branch>` | `git worktree add` per repo; default setup; `--from`, `--no-setup` |
| `mwt rm <branch>` | Remove worktrees; `--force` if dirty/residual |
| `mwt list` | Aggregate worktrees; `--branch` filter; `--format <table\|json>` |
| `mwt path <branch> <repo>` | Print absolute worktree path (for Agents / scripts); `--format <table\|json>` |
| `mwt setup <branch>` | Re-run setup on existing worktrees |
| `mwt doctor` | Report prunable / unregistered / missing main / setup_missing; `--fix` re-runs setup for all setup_missing only; `--format <table\|json>` |
| `mwt version` | Binary version (`--version` / `-v` also work) |
| `mwt skill install` | Install this skill into `~/.agents/skills/mwt` |

Shared flags: `--repos`, `--continue` (`doctor --fix` also accepts `--continue`).

## Configuration (`.mwt.yaml`)

- Found by walking **up** from cwd (except `init`, which writes in cwd).
- `repos`: main-checkout paths relative to meta-root (also `{{REPO}}`).
- `worktree_path`: template. `init` fills §5.1 dual-default:
  - meta-root has `.git` → `.worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}`
  - else → `worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}`
- `setup`: ordered `copy` / `run` steps (placeholders like `{{WORKTREE_PATH}}`).

## Placeholders

| Placeholder | Meaning | `worktree_path` | `setup` |
|-------------|---------|-----------------|---------|
| `{{ROOT}}` | Absolute meta-root | yes | yes |
| `{{REPO}}` | Current `repos` entry (rel. meta-root) | yes | yes |
| `{{REPO_PATH}}` | Alias of `{{REPO}}` | yes | yes |
| `{{MAIN_PATH}}` | Absolute main checkout | yes | yes |
| `{{BRANCH}}` | Target branch name | yes | yes |
| `{{WORKTREE_PATH}}` | Absolute worktree path | no (self-ref) | yes |
| `{{WORKTREE_NAME}}` | Basename of worktree path | no (depends on above) | yes |

Form `{{NAME}}`, uppercase; unknown placeholders fail. `setup` fields (`from`/`to`/`command`/`dir`) expand per the `setup` column.

## Agent workflow

1. Prefer editing inside worktrees: `mwt path <branch> <repo>` — do **not** dirty main checkouts for multi-repo tasks.
2. Create set: `mwt add <branch> --from <base>` (or `--repos` subset).
3. After `--no-setup`, use `mwt setup <branch>`.
4. Clean up: `mwt rm <branch>`; use `mwt doctor` if paths drift.

## Details

Full design: repo doc `docs/20260717-113800-mwt-multi-repo-worktree-cli.md`.
