#!/usr/bin/env sh
# Case: existing dest is refused without --force.
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_refuse_existing() {
	export MWT_SKILL_DIR="$CURRENT_TMP/skills"
	# Pre-create the dest with sentinel content
	mkdir -p "$MWT_SKILL_DIR/mwt"
	printf 'old\n' >"$MWT_SKILL_DIR/mwt/SKILL.md"
	run_script
	[ "$_RC" != "0" ] || {
		echo "expected non-zero rc" >&2
		return 1
	}
	last_stderr | grep -q "already exists" || {
		echo "expected 'already exists' in stderr; got: $(last_stderr)" >&2
		return 1
	}
	# Sentinel content must be unchanged
	body=$(cat "$MWT_SKILL_DIR/mwt/SKILL.md")
	[ "$body" = "old" ] || {
		echo "sentinel was modified: $body" >&2
		return 1
	}
}

run_case "refuses_when_exists_without_force" test_refuse_existing
exit 0
