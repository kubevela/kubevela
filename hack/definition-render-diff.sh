#!/usr/bin/env bash
# Lists the shipped definitions that render differently between an earlier release and this
# tree, for the release notes: what an Application's workload changes to on its first edit
# after an upgrade.
#
# The earlier release's definitions are laid beside this tree's test files and rendered with
# this tree's renderer, so a difference is the definition's, not the renderer's.
#
# Usage: hack/definition-render-diff.sh [-v] <git-ref>     e.g. v1.11.0
#   -v   also print a unified diff of each differing render (earlier release first)
# Needs envtest assets in KUBEBUILDER_ASSETS for cases that seed a cluster (make envtest).
set -euo pipefail

usage() {
	echo "usage: $0 [-v] <git-ref>" >&2
	exit 2
}
verbose=false
if [ "${1:-}" = "-v" ]; then
	verbose=true
	shift
fi
[ $# -eq 1 ] || usage
ref="$1"

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
git rev-parse --verify --quiet "$ref^{commit}" >/dev/null || {
	echo "error: $ref is not a commit in this repository" >&2
	exit 1
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
git archive "$ref" vela-templates/definitions/internal | tar -x -C "$tmp"
old="$tmp/vela-templates/definitions/internal"
current="vela-templates/definitions/internal"

# The earlier release's own tests are not the ones the golden renders came from.
find "$old" -name '*_test.cue' -exec rm -f {} +

added=""
added_count=0
compared=0
for t in $(cd "$current" && find . -name '*_test.cue' | sort); do
	def="${t%_test.cue}.cue"
	if [ -f "$old/$def" ]; then
		cp "$current/$t" "$old/$t"
		compared=$((compared + 1))
	else
		added="${added}- ${def#./}: new in this release"$'\n'
		added_count=$((added_count + 1))
	fi
done

report="$tmp/report"
renders="$tmp/renders"
if ! out="$(GOLDEN_DEFINITIONS_DIR="$old" GOLDEN_REPORT="$report" GOLDEN_RENDERS_DIR="$renders" VELA_SHIPPED_DEFINITION_TESTS=1 \
	go test -count=1 ./pkg/definition/cuetest/ -run TestShippedDefinitionsMatchTheirGoldenRenders 2>&1)"; then
	echo "error: rendering $ref's definitions failed:" >&2
	echo "$out" >&2
	exit 1
fi

differs="$(grep -c . "$report" || true)"
grep . "$report" || true
if $verbose && [ -d "$renders" ]; then
	golden="pkg/definition/cuetest/testdata/golden"
	(cd "$renders" && find . -name '*.yaml' | sort) | while read -r f; do
		diff -u --label "$ref ${f#./}" --label "current ${f#./}" "$renders/$f" "$golden/$f" || true
	done
fi
printf '%s' "$added"
echo "summary: $compared definitions compared with $ref, $differs cases differ, $added_count definitions new"
