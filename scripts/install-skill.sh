#!/usr/bin/env sh
# scripts/install-skill.sh — install the mwt Agent skill into a skills dir.
#
# 与二进制完全解耦:skill 资产来自仓库 `skills/mwt/` 目录,不嵌入二进制。
# 默认目标 `~/.agents/skills/mwt`,可通过 `--dir` 或 `MWT_SKILL_DIR` 改写。
#
# Usage:
#   scripts/install-skill.sh                # install to ~/.agents/skills/mwt
#   scripts/install-skill.sh --force        # overwrite existing mwt skill
#   scripts/install-skill.sh --dir PATH     # install to PATH/mwt
#   scripts/install-skill.sh --dry-run      # print the plan only
#   MWT_SKILL_DIR=PATH scripts/install-skill.sh
#
# 行为对齐 Go 版 `mwt-legacy/internal/skilldata/sync.go` (1:1 复刻)。

set -eu

# 解析脚本自身位置 → 定位仓库根 → 验证 skill 源存在。
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
SRC="$REPO_ROOT/skills/mwt"

die() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

[ -d "$SRC" ] || die "source skill not found at $SRC"
[ -f "$SRC/SKILL.md" ] || die "SKILL.md missing in $SRC"

# 默认父目录;尊重 $MWT_SKILL_DIR 覆盖。
if [ -n "${MWT_SKILL_DIR:-}" ]; then
	dir="$MWT_SKILL_DIR"
else
	dir="${HOME:-}/.agents/skills"
	if [ -z "$dir" ] || [ "$dir" = "/.agents/skills" ]; then
		die "HOME is unset and MWT_SKILL_DIR not provided"
	fi
fi
force=0
dry_run=0

while [ $# -gt 0 ]; do
	case "$1" in
	--dir)
		[ $# -ge 2 ] || die "--dir requires a value"
		dir=$2
		shift 2
		;;
	--dir=*)
		dir=${1#--dir=}
		shift
		;;
	--force)
		force=1
		shift
		;;
	--dry-run)
		dry_run=1
		shift
		;;
	-h | --help)
		sed -n '3,13p' "$0"
		exit 0
		;;
	*)
		die "unknown arg: $1"
		;;
	esac
done

# 绝对化(若已是绝对路径则不动)。`cd` 容错:目标不存在时退化为
# 拼接,不报错——后续 `mkdir -p` 会创建。
case "$dir" in
/*) abs_dir=$dir ;;
*) abs_dir=$PWD/$dir ;;
esac
# 解析 `..` / `.` 让 `dest` 与人类可读形式对齐。
if command -v realpath >/dev/null 2>&1; then
	dest=$(realpath -m "$abs_dir") || dest=$abs_dir
else
	dest=$abs_dir
fi
dest=$dest/mwt

# 存在性检查
if [ -e "$dest" ]; then
	if [ ! -d "$dest" ]; then
		die "$dest exists and is not a directory"
	fi
	if [ "$force" -ne 1 ]; then
		die "$dest already exists (use --force to overwrite)"
	fi
	if [ "$dry_run" -ne 1 ]; then
		rm -rf -- "$dest"
	fi
fi

# 复制
if [ "$dry_run" -eq 1 ]; then
	printf 'plan: mkdir -p %s\n' "$dest"
	find "$SRC" -mindepth 1 -maxdepth 1 ! -name . -print | while IFS= read -r entry; do
		[ -n "$entry" ] || continue
		printf 'plan: copy %s -> %s/\n' "$entry" "$dest"
	done
else
	mkdir -p -- "$dest"
	# 用 tar -cf - | tar -xf - 跨 BSD/GNU 都支持,保留权限。
	if command -v tar >/dev/null 2>&1; then
		(cd "$SRC" && tar -cf - .) | (cd "$dest" && tar -xf -)
	else
		# 极少见(无 tar)的后备:逐文件 cp。POSIX 保证 cp 可用。
		cp -R "$SRC"/. "$dest"/
	fi
fi

printf 'synced skill to %s\n' "$dest"
