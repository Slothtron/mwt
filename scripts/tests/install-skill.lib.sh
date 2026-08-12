#!/usr/bin/env sh
# scripts/tests/install-skill.sh — 共享的测试引导器。
#
# 提供 `setup` / `teardown` / `assert_*` 工具,被同目录下的 `*.case.sh` 测试
# 入口用 `.` source 后,再 source 各 `*.case.sh`。
#
# 设计:不依赖 bats-core;每个 case 是一个普通 POSIX sh 脚本,直接被
# `mise run test-shell` 顺序执行。失败时 `exit 1` 中断整个运行链。

set -eu

# 定位仓库根(测试脚本在 scripts/tests/ 下)。
TEST_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/../.." && pwd)
SCRIPT="$REPO_ROOT/scripts/install-skill.sh"

# --- 测试夹具 ---
TMPDIR_ROOT=$(mktemp -d)
export HOME="$TMPDIR_ROOT/home"
mkdir -p "$HOME"

PASS=0
FAIL=0

case_color_green() { printf '\033[32m%s\033[0m' "$1"; }
case_color_red() { printf '\033[31m%s\033[0m' "$1"; }

# 每次 case 入口调用,提供干净的 TMP 与 fresh HOME。
#
# 默认行为:不预设 MWT_SKILL_DIR,让脚本走 `$HOME/.agents/skills`。
# 需要测 MWT_SKILL_DIR 路径的 case 在自己体内 `export MWT_SKILL_DIR=...`。
setup() {
	CURRENT_TMP=$(mktemp -d "$TMPDIR_ROOT/case.XXXXXX")
	unset MWT_SKILL_DIR 2>/dev/null || true
}

teardown() {
	if [ -n "${CURRENT_TMP:-}" ] && [ -d "$CURRENT_TMP" ]; then
		rm -rf "$CURRENT_TMP"
	fi
	unset CURRENT_TMP MWT_SKILL_DIR
}

# 包装一个 case 名称 + 实际测试体。运行体应 `exit 0` 成功 / `exit 1` 失败。
run_case() {
	name=$1
	shift
	setup
	if ("$@") 2>&1; then
		case_color_green "ok  " >&2
		printf '%s\n' "$name" >&2
		PASS=$((PASS + 1))
	else
		rc=$?
		case_color_red "FAIL" >&2
		printf ' %s (rc=%d)\n' "$name" "$rc" >&2
		FAIL=$((FAIL + 1))
	fi
	teardown
}

# --- 断言工具(直接被测试体调用) ---
assert_file_exists() {
	[ -f "$1" ] || {
		echo "expected file: $1" >&2
		return 1
	}
}

assert_dir_exists() {
	[ -d "$1" ] || {
		echo "expected dir: $1" >&2
		return 1
	}
}

assert_file_absent() {
	[ ! -e "$1" ] || {
		echo "expected absent: $1" >&2
		return 1
	}
}

assert_file_contains() {
	file=$1
	needle=$2
	grep -q -F -- "$needle" "$file" || {
		echo "expected '$needle' in $file" >&2
		return 1
	}
}

assert_dir_empty() {
	dir=$1
	# set -- "$dir"/* 会在 dir 为空时 echo 字面 "dir/*" → 1 个参数
	# set -- ... 在非空时为 0+ 个真实条目。配合 [ -e "$dir/$_" ] 太脆弱;
	# 用 find 最稳。
	if [ -d "$dir" ] && [ -z "$(find "$dir" -mindepth 1 -maxdepth 1 2>/dev/null)" ]; then
		return 0
	fi
	echo "expected empty dir: $dir" >&2
	return 1
}

# 跑一次脚本,捕获 stdout / stderr 与 exit code。
run_script() {
	# 用 _OUT/_ERR/_RC 暴露给调用者。POSIX 不支持 stdout/stderr 分别重定向
	# 进子 shell 之外的变量,改用临时文件。
	_OUT_FILE=$CURRENT_TMP/.out
	_ERR_FILE=$CURRENT_TMP/.err
	if "$SCRIPT" "$@" >"$_OUT_FILE" 2>"$_ERR_FILE"; then
		_RC=0
	else
		_RC=$?
	fi
}

last_stdout() { cat "$CURRENT_TMP/.out"; }
last_stderr() { cat "$CURRENT_TMP/.err"; }
last_rc() { printf '%s' "$_RC"; }
