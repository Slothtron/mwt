#!/usr/bin/env sh
# scripts/tests/install-skill.test.sh — 顺序执行所有 install-skill.*.case.sh。
#
# 每个 case 内部已经自己 assert + 计数;这里只负责分发 + 汇总退出码。
# 任何一个 case 失败 → 整个 runner 退出非零,中断 mise run test-shell。

set -eu

TEST_DIR=$(cd "$(dirname "$0")" && pwd)

total=0
failed=0
for case in "$TEST_DIR"/install-skill.*.case.sh; do
	[ -f "$case" ] || continue
	total=$((total + 1))
	name=$(basename "$case")
	printf '==> %s\n' "$name"
	if ! sh "$case"; then
		failed=$((failed + 1))
		printf 'FAIL: %s\n' "$name" >&2
	fi
done

echo
if [ "$failed" -eq 0 ]; then
	printf 'all %d cases passed\n' "$total"
	exit 0
else
	printf '%d/%d cases failed\n' "$failed" "$total" >&2
	exit 1
fi
