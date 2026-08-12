#!/usr/bin/env sh
# Case: dest is a regular file (not a dir) → reject.
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_path_is_file() {
	export MWT_SKILL_DIR="$CURRENT_TMP/skills"
	mkdir -p "$MWT_SKILL_DIR"
	printf 'blocked\n' >"$MWT_SKILL_DIR/mwt"
	run_script
	[ "$_RC" != "0" ] || {
		echo "expected non-zero rc" >&2
		return 1
	}
	last_stderr | grep -q "not a directory" || {
		echo "expected 'not a directory' in stderr; got: $(last_stderr)" >&2
		return 1
	}
	# File should still be present and untouched
	[ "$(cat "$MWT_SKILL_DIR/mwt")" = "blocked" ] || return 1
}

run_case "refuses_when_dest_is_regular_file" test_path_is_file
exit 0
