#!/usr/bin/env sh
# Case: MWT_SKILL_DIR env var overrides default (and is overridden by --dir).
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_env_var() {
	# setup() does not pre-set MWT_SKILL_DIR; we export it here.
	export MWT_SKILL_DIR="$CURRENT_TMP/skills"
	run_script
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC" >&2
		return 1
	}
	assert_file_exists "$MWT_SKILL_DIR/mwt/SKILL.md"
	# Default HOME must NOT have been touched
	assert_file_absent "$HOME/.agents/skills/mwt/SKILL.md"
}

test_cli_overrides_env() {
	export MWT_SKILL_DIR="$CURRENT_TMP/should_not_be_used"
	other="$CURRENT_TMP/other"
	run_script --dir "$other"
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC" >&2
		return 1
	}
	assert_file_exists "$other/mwt/SKILL.md"
	# Env path was not used
	assert_file_absent "$MWT_SKILL_DIR/mwt/SKILL.md"
}

run_case "respects_MWT_SKILL_DIR" test_env_var
run_case "--dir_overrides_MWT_SKILL_DIR" test_cli_overrides_env
exit 0
