#!/usr/bin/env sh
# Case: default dir creates mwt skill under HOME/.agents/skills
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_default_install() {
	run_script
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC stderr=$(last_stderr)" >&2
		return 1
	}
	assert_dir_exists "$HOME/.agents/skills/mwt"
	assert_file_exists "$HOME/.agents/skills/mwt/SKILL.md"
	# output must mention dest
	last_stdout | grep -q "synced skill to $HOME/.agents/skills/mwt" || {
		echo "missing 'synced skill to' line" >&2
		return 1
	}
}

run_case "default_install_creates_under_home_agents_skills" test_default_install
exit 0
