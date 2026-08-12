#!/usr/bin/env sh
# Case: --dry-run prints plan without writing.
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_dry_run() {
	export MWT_SKILL_DIR="$CURRENT_TMP/skills"
	run_script --dry-run
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC" >&2
		return 1
	}
	last_stdout | grep -q "plan:" || {
		echo "expected 'plan:' lines" >&2
		return 1
	}
	# dest must not exist
	assert_file_absent "$MWT_SKILL_DIR/mwt"
	assert_file_absent "$MWT_SKILL_DIR/mwt/SKILL.md"
}

test_dry_run_with_existing() {
	export MWT_SKILL_DIR="$CURRENT_TMP/skills"
	# Pre-existing dir; --dry-run must not delete it.
	mkdir -p "$MWT_SKILL_DIR/mwt"
	printf 'sentinel\n' >"$MWT_SKILL_DIR/mwt/SKILL.md"
	run_script --dry-run --force
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC stderr=$(last_stderr)" >&2
		return 1
	}
	[ "$(cat "$MWT_SKILL_DIR/mwt/SKILL.md")" = "sentinel" ] || {
		echo "sentinel was deleted under --dry-run" >&2
		return 1
	}
}

run_case "dry_run_does_not_write" test_dry_run
run_case "dry_run_does_not_delete_existing" test_dry_run_with_existing
exit 0
