#!/usr/bin/env sh
# Case: --help and unknown args.
# shellcheck disable=SC1091,SC2329
. "$(dirname "$0")/install-skill.lib.sh"

test_help() {
	run_script --help
	[ "$_RC" = "0" ] || {
		echo "rc=$_RC" >&2
		return 1
	}
	last_stdout | grep -q "Usage:" || {
		echo "missing Usage:" >&2
		return 1
	}
	last_stdout | grep -q "MWT_SKILL_DIR" || {
		echo "missing env hint" >&2
		return 1
	}
}

test_unknown_arg() {
	run_script --bogus
	[ "$_RC" != "0" ] || {
		echo "expected non-zero rc" >&2
		return 1
	}
	last_stderr | grep -q "unknown arg: --bogus" || {
		echo "expected 'unknown arg' in stderr" >&2
		return 1
	}
}

run_case "--help_prints_usage" test_help
run_case "unknown_arg_is_rejected" test_unknown_arg
exit 0
