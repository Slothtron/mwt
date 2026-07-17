# mwt — Multi-repo WorkTrees

polyrepo git worktree manager：按一份 `.mwt.yaml` 对多个独立 Git 仓批量创建 / 清理 / 巡检同逻辑分支的 worktree，并跑最小本地 setup。

## Install

需要本机已安装 Go 与 `git`。

```bash
# 从模块安装（发布后）
go install github.com/Slothtron/mwt/cmd/mwt@latest

# 或从源码
git clone https://github.com/Slothtron/mwt.git
cd mwt
go install ./cmd/mwt

# 可选：注入版本号（发布构建）
go build -ldflags "-X github.com/Slothtron/mwt/internal/version.Version=0.1.0 -X github.com/Slothtron/mwt/internal/version.Commit=$(git rev-parse --short HEAD)" -o mwt ./cmd/mwt
```

二进制进入 `$GOBIN` / `$GOPATH/bin`，确保该目录在 `PATH` 中。使用方仓库**不**提交 `./bin/mwt` 或嵌入 `tools/mwt`。用 `mwt version` 或 `mwt --version` 查看版本。

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
| `mwt list` | 聚合各仓 worktree；可按 `--branch` 过滤 |
| `mwt path <branch> <repo>` | 打印绝对路径（模板渲染，不要求目录已存在） |
| `mwt setup <branch>` | 对已有 worktree 补跑 setup |
| `mwt doctor` | 巡检 prunable / 未注册目录 / 主检出缺失等，只报告不自动删 |

常用 flags：`--repos`（子集）、`--continue`（某仓失败后继续，仍非 0 退出）。

## Configuration

配置文件名为 **`.mwt.yaml`**。`mwt init` 在 **cwd** 生成；其它命令从当前目录**向上**查找。主要字段：

| 字段 | 说明 |
|------|------|
| `root` | 相对配置文件的元根，默认 `.` |
| `repos` | 主检出相对元根的路径列表（字符串数组） |
| `worktree_path` | 路径模板；`mwt init` 会按双默认显式写入；**省略时** Load 仍按下方规则选前缀 |
| `setup` | 有序步骤列表（v1：`copy` / `run`） |

完整字段与占位符见技术方案 `docs/20260717-113800-mwt-multi-repo-worktree-cli.md` §6。

### Dual-default path rule

仅当配置**未写** `worktree_path` 时生效：

| 条件 | 缺省 `worktree_path` |
|------|----------------------|
| 元根存在 `{ROOT}/.git`（目录或 gitfile） | `.worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}` |
| 元根不存在 `.git` | `worktrees/{{REPO}}/{{BRANCH}}/{{REPO}}` |

显式写出的 `worktree_path`（无论 `worktrees/...` 还是 `.worktrees/...`）**原样使用**，不会按 `.git` 自动改写前缀。

**Git 元根建议：** 将 `.worktrees/` 加入该仓 `.gitignore`，避免点目录下的 worktree 污染主工作区视图与误提交。

## vs workmux

| | [workmux](https://github.com/raine/workmux) | mwt |
|--|--|--|
| 模型 | 1 repo × N branches × tmux windows | N repos × 1 branch set |
| 焦点 | 单仓 + tmux 并行 | 多独立仓同分支联调 |
| v1 | tmux 为核心叙事 | **不做** tmux / TUI / Agent |

单仓日常 TUI 仍可用 lazygit；跨仓长期同分支联调用 mwt。

## Non-goals (v1)

- 不替代 `git`、不实现自研 VCS
- 不把多仓合并为单一 monorepo 根
- 不生成 `.code-workspace`，不对接 Cursor Worktree
- 不自动打开 IDE、不自动建远程 PR
- 不绑定 tmux / WezTerm
- setup 不跑 DB migrate / 全量测试 / 起中间件
- 不嵌入使用方仓库（无 `tools/mwt`、无 `./bin/mwt`）

## License

见仓库许可文件（若有）。
