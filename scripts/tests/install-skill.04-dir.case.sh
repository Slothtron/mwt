#!/usr/bin/env sh
# Case: --dir overrides default.
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_dir_override() {
	custom="$CURRENT_TMP/custom"
	run_script --dir "$custom"
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC stderr=$(last_stderr)" >&2
		return 1
	}
	assert_file_exists "$custom/mwt/SKILL.md"
	# Default HOME/.agents must NOT have been touched
	assert_file_absent "$HOME/.agents/skills/mwt/SKILL.md"
}

test_dir_equals_form() {
	custom="$CURRENT_TMP/custom2"
	run_script --dir="$custom"
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC stderr=$(last_stderr)" >&2
		return 1
	}
	assert_file_exists "$custom/mwt/SKILL.md"
}

run_case "--dir_overrides_default" test_dir_override
run_case "--dir_equals_form_accepted" test_dir_equals_form
exit 0
