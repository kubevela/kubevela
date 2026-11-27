#!/usr/bin/env bash
# Tests hack/preupgrade-check-1.12.sh against the captured clusters in
# hack/testdata/preupgrade-check-1.12.
#
# Each scenario directory is laid over base/ (its files replace base's; names listed in an
# `absent` file are removed) and checked with --from-dir. Its `expect` file holds the findings
# expected beyond base's, one `level check object` per line, plus optional lines:
#   exit N          the expected exit code (default 0)
#   match REGEX     a line the text output must contain
#   alone           base's findings are not expected, only the scenario's
#
# Usage: hack/preupgrade-check-1.12_test.sh [-v] [scenario...]
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
check="$here/preupgrade-check-1.12.sh"
data="$here/testdata/preupgrade-check-1.12"
verbose=false
if [ "${1:-}" = "-v" ]; then
	verbose=true
	shift
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

passed=0
failed=0
fail() {
	failed=$((failed + 1))
	echo "FAIL $1: $2"
}

run_scenario() {
	local name="$1" dir="$data/$1" work="$tmp/$1"
	mkdir -p "$work"
	cp "$data/base/"*.json "$work/"
	cp "$dir/"*.json "$work/" 2>/dev/null || true
	if [ -f "$dir/absent" ]; then
		while read -r f; do rm -f "$work/$f"; done <"$dir/absent"
	fi

	local want_exit=0 got_exit=0
	want_exit="$(sed -n 's/^exit //p' "$dir/expect")"
	want_exit="${want_exit:-0}"
	{
		grep -qx alone "$dir/expect" || grep -v '^exit \|^match ' "$data/base/expect"
		grep -v '^exit \|^match \|^alone$' "$dir/expect" || true
	} |
		sed '/^$/d' | sort >"$work/want"

	"$check" --from-dir "$work" --json >"$work/out.json" 2>"$work/err" || got_exit=$?
	jq -r '.findings[] | "\(.level) \(.check) \(.object)"' "$work/out.json" 2>/dev/null | sort >"$work/got" || true

	local ok=true
	if [ "$got_exit" != "$want_exit" ]; then
		fail "$name" "exit $got_exit, want $want_exit $(cat "$work/err")"
		ok=false
	fi
	if ! diff -u "$work/want" "$work/got" >"$work/diff"; then
		fail "$name" "findings differ (- want, + got):"
		sed -n '3,$p' "$work/diff" | grep '^[-+]' | sed 's/^/    /'
		ok=false
	fi
	# Every finding carries a problem, a fix and a link into the upgrade guide, and the summary counts them.
	local bad
	bad="$(jq -r '
		([.findings[] | select(
			(.level | IN("error", "warn", "info") | not)
			or ((.problem // "") | type != "string" or . == "")
			or ((.fix // "") | type != "string" or . == "")
			or ((.link // "") | type != "string" or (startswith("https://kubevela.io/docs/upgrade-guides/v1.12/#") | not)))
		| "finding \(.level) \(.check) \(.object) lacks a problem, fix or upgrade-guide link"]
		+ (.findings as $f | {errors: ([$f[] | select(.level == "error")] | length),
			warnings: ([$f[] | select(.level == "warn")] | length), info: ([$f[] | select(.level == "info")] | length)}
			| if . == $s then [] else ["summary \($s | tojson), the findings count \(tojson)"] end)) | .[]
	' --argjson s "$(jq -c '.summary' "$work/out.json" 2>/dev/null || echo null)" "$work/out.json" 2>&1)" || bad="unreadable JSON output: $bad"
	if [ -n "$bad" ]; then
		fail "$name" "$bad"
		ok=false
	fi
	local text_exit=0
	"$check" --from-dir "$work" >"$work/out.txt" 2>&1 || text_exit=$?
	while read -r regex; do
		if ! grep -Eq -- "$regex" "$work/out.txt"; then
			fail "$name" "text output has no line matching: $regex"
			ok=false
		fi
	done < <(sed -n 's/^match //p' "$dir/expect")
	if [ "$text_exit" != "$got_exit" ]; then
		fail "$name" "text run exited $text_exit, the JSON run $got_exit"
		ok=false
	fi
	if $ok; then
		passed=$((passed + 1))
		if $verbose; then echo "ok   $name"; fi
	elif $verbose; then
		sed 's/^/    /' "$work/out.txt"
	fi
}

if [ $# -gt 0 ]; then
	scenarios="$*"
else
	scenarios="$(cd "$data" && for d in */; do [ "$d" = base/ ] || echo "${d%/}"; done)"
fi
for s in $scenarios; do
	run_scenario "$s"
done

# The command line: a missing directory cannot run, -q prints only the summary.
code=0
"$check" --from-dir "$tmp/none" >/dev/null 2>&1 || code=$?
if [ "$code" = 2 ]; then passed=$((passed + 1)); else fail "cli" "--from-dir on a missing directory exited $code, want 2"; fi
mkdir -p "$tmp/quiet"
cp "$data/base/"*.json "$tmp/quiet/"
quiet="$("$check" --from-dir "$tmp/quiet" -q)"
if [ "$quiet" = "0 errors, 0 warnings, 3 info" ]; then passed=$((passed + 1)); else fail "cli" "-q printed: $quiet"; fi
# Counts of one are singular, in the summary and in -v's per-check lines.
mkdir -p "$tmp/one"
cp "$data/base/"*.json "$tmp/one/"
cp "$data/pinned-revisions/"*.json "$tmp/one/"
quiet="$("$check" --from-dir "$tmp/one" -q || true)"
if [ "$quiet" = "1 error, 1 warning, 3 info" ]; then passed=$((passed + 1)); else fail "cli" "-q printed: $quiet"; fi
verbose_out="$("$check" --from-dir "$tmp/quiet" -v)"
if echo "$verbose_out" | grep -qx 'check helm-values: 1 finding' && echo "$verbose_out" | grep -qx 'check trait-conflicts: 0 findings'; then
	passed=$((passed + 1))
else
	fail "cli" "-v does not count one finding as 1 finding: $(echo "$verbose_out" | grep '^check helm-values')"
fi

# --resolve with --from-dir resolves the captured hosts afresh, a Secret's host as well as a literal
# one. A stub getent answers every name with the metadata address.
mkdir -p "$tmp/stub" "$tmp/resolve"
printf '#!/bin/sh\necho "169.254.169.254 STREAM $2"\n' >"$tmp/stub/getent"
chmod +x "$tmp/stub/getent"
cp "$data/base/"*.json "$tmp/resolve/"
cp "$data/resolve-secret/"*.json "$tmp/resolve/"
PATH="$tmp/stub:$PATH" "$check" --from-dir "$tmp/resolve" --resolve --json 2>/dev/null >"$tmp/resolve/out.json" || true
for app in hook-resolves-to-metadata call-resolves-to-metadata; do
	if jq -e --arg o "Application default/$app" \
		'any(.findings[]; .check == "http-guard" and .object == $o and (.problem | test("resolves to 169.254.169.254")))' \
		"$tmp/resolve/out.json" >/dev/null 2>&1; then
		passed=$((passed + 1))
	else
		fail "resolve" "--resolve found no metadata address for $app"
	fi
done

echo "$passed passed, $failed failed"
[ "$failed" -eq 0 ]
