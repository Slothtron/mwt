# mwt — Multi-repo WorkTrees

polyrepo git worktree manager：按一份 `.mwt.yaml` 对多个独立 Git 仓批量创建 / 清理 / 巡检同逻辑分支的 worktree，并跑最小本地 setup。

## Install

需要本机已安装 `cargo`（与 `mise`）、`git`。开发态默认使用 `mise.toml` 钉定的 `rust = 1.96.1`。

```bash
# 从源码（开发态）
git clone https://github.com/Slothtron/mwt.git
cd mwt
mise install           # 拉取钉定的 rust
mise run build         # cargo build
./target/debug/mwt --version

# 或从源码（一次性发布构建）
cargo build --release
./target/release/mwt --version
```

二进制在 `./target/{debug,release}/mwt`，把它加到 `PATH` 或拷到 `~/.local/bin/`。

> **BREAKING（相对 v0.x Go 版）**：`mwt skill sync` 子命令已删除，改为 `mwt skill install`（纯 Rust，不嵌入二进制）或仓库脚本 `scripts/install-skill.sh`，二者行为一致：

```bash
# 默认装到 ~/.agents/skills/mwt（二进制子命令）
mwt skill install

# 覆盖已存在的目录
mwt skill install --force

# 装到自定义父目录 / 只打印计划 / JSON 输出
mwt skill install --dir /path/to/skills
mwt skill install --dry-run
mwt skill install --format json

# 或使用仓库脚本（等价）
./scripts/install-skill.sh
./scripts/install-skill.sh --force
./scripts/install-skill.sh --dir /path/to/skills
./scripts/install-skill.sh --dry-run
MWT_SKILL_DIR=/path/to/skills ./scripts/install-skill.sh
```

## Quick start

在元根（含各仓主检出的目录）生成或放置 `.mwt.yaml`，然后：

```bash
# 自动扫描当前目录及子目录中的 Git 仓（默认最多 10 层），写入 .mwt.yaml
mwt init

mwt doctor
mwt add my-feature --from master
mwt list --branch my-feature
mwt path my-feature <repo>
mwt rm my-feature
```

也可手写配置；示例见 [`examples/`](examples/)。

## Commands

| 命令 | 作用 |
|------|------|
| `mwt init` | 向下扫描 Git 主检出并生成 `.mwt.yaml`（按元根是否有 `.git` 写入 `worktree_path`）；`--depth`（默认 10）、`--dry-run`、`--force` |
| `mwt version` | 打印 mwt 二进制版本（亦支持 `mwt --version` / `-v`） |
| `mwt add <branch>` | 对各仓 `git worktree add`；可选 `--from` 建分支；默认跑 setup；`--no-setup` 跳过 |
| `mwt rm <branch>` | 移除各仓 worktree；脏/残留可用 `--force` |
| `mwt list` | 聚合各仓 worktree；可按 `--branch` 过滤；`--format <table\|json>` |
| `mwt path <branch> <repo>` | 打印绝对路径（模板渲染，不要求目录已存在）；`--format <table\|json>` |
| `mwt setup <branch>` | 对已有 worktree 补跑 setup |
| `mwt doctor` | 巡检 prunable / 未注册目录 / 主检出缺失 / setup 缺失等；`--fix` 仅自动补跑 `setup_missing`（不 prune、不删目录）；`--format <table\|json>`；未装 skill 或版本不符时 stderr 打印 `hint:` |
| `mwt skill install` | 安装 mwt Agent skill 到 `~/.agents/skills/mwt`（`--dir`、`--source`、`--force`、`--dry-run`、`--format <table\|json>`）；源取 `--source` > `MWT_SKILL_SOURCE` > `./skills/mwt` |

常用 flags：`--repos`（子集）、`--continue`（某仓失败后继续，仍非 0 退出；`doctor --fix` 亦支持）。

## Configuration

配置文件名为 **`.mwt.yaml`**。`mwt init` 在 **cwd** 生成；其它命令从当前目录**向上**查找。主要字段：

| 字段 | 说明 |
|------|------|
| `root` | 相对配置文件的元根，默认 `.` |
| `repos` | 主检出相对元根的路径列表（字符串数组） |
| `worktree_path` | 路径模板；`mwt init` 会按双默认显式写入；**省略时** Load 仍按下方规则选前缀 |
| `setup` | 有序步骤列表（v1：`copy` / `run`） |

动作字段、`skip_*` 等细节见技术方案 `docs/20260717-113800-mwt-multi-repo-worktree-cli.md` §6。

### Placeholders

| 占位符 | 含义 | `worktree_path` | `setup` |
|--------|------|-----------------|---------|
| `{{ROOT}}` | 元根绝对路径 | 是 | 是 |
| `{{REPO}}` | 当前 repos 项（相对元根） | 是 | 是 |
| `{{REPO_PATH}}` | 与 `{{REPO}}` 等价 | 是 | 是 |
| `{{MAIN_PATH}}` | 主检出绝对路径 | 是 | 是 |
| `{{BRANCH}}` | 目标分支名 | 是 | 是 |
| `{{WORKTREE_PATH}}` | 本仓 worktree 绝对路径 | 否（防自引用） | 是 |
| `{{WORKTREE_NAME}}` | worktree 目录 basename | 否（依赖上者） | 是 |

形式 `{{NAME}}`，NAME 全大写；未知占位符报错。`setup` 的 `from`/`to`/`command`/`dir` 等字段按上表 `setup` 列展开。

### Dual-default path rule

仅当配置**未写** `worktree_path` 时生效：

| 条件 | 缺省 `worktree_path` |
|------|----------------------|
| 元根存在 `{ROOT}/.git`（目录或 gitfile） | `.worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}` |
| 元根不存在 `.git` | `worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}` |

显式写出的 `worktree_path`（无论 `worktrees/...` 还是 `.worktrees/...`）**原样使用**，不会按 `.git` 自动改写前缀。

**Git 元根建议：** 将 `.worktrees/` 加入该仓 `.gitignore`，避免点目录下的 worktree 污染主工作区视图与误提交。

## Non-goals (v1)

- 不替代 `git`、不实现自研 VCS
- 不把多仓合并为单一 monorepo 根
- 不生成 `.code-workspace`，不对接 Cursor Worktree
- 不自动打开 IDE、不自动建远程 PR
- 不绑定 tmux / WezTerm
- setup 不跑 DB migrate / 全量测试 / 起中间件
- 不嵌入使用方仓库（无 `tools/mwt`、无 `./bin/mwt`）

## Development

`mise.toml` 钉定 `rust = 1.96.1`，并暴露下列 task：

| Task | 作用 |
|------|------|
| `mise run build` | `cargo build` |
| `mise run test` | `cargo test` |
| `mise run test-shell` | 跑 `scripts/tests/install-skill.*.case.sh`（POSIX sh 自跑测试，无 bats 依赖） |
| `mise run check` | `cargo clippy --all-targets -- -D warnings` |
| `mise run lint-shell` | 对 `scripts/*.sh` 跑 `shellcheck` + `shfmt -d`（未安装则跳过） |
| `mise run fmt` | `cargo fmt --all` |

仓库静态资产 `skills/mwt/SKILL.md` 与二进制完全解耦：二进制无 `include_str!` / `include_dir!` / `vergen`，`mwt --help` 不含 `skill` 子命令。

## License

见仓库许可文件（若有）。
