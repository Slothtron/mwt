#!/usr/bin/env sh
# Case: --force overwrites existing dest.
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_force_overwrite() {
	export MWT_SKILL_DIR="$CURRENT_TMP/skills"
	mkdir -p "$MWT_SKILL_DIR/mwt"
	printf 'old\n' >"$MWT_SKILL_DIR/mwt/SKILL.md"
	run_script --force
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC stderr=$(last_stderr)" >&2
		return 1
	}
	# Content must now be the real SKILL.md (not 'old').
	body=$(cat "$MWT_SKILL_DIR/mwt/SKILL.md")
	[ "$body" != "old" ] || {
		echo "old sentinel still present" >&2
		return 1
	}
	assert_file_contains "$MWT_SKILL_DIR/mwt/SKILL.md" "name: mwt"
}

run_case "force_overwrites_existing_dir" test_force_overwrite
exit 0
