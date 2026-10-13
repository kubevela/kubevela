#!/usr/bin/env bash
# Exercise real fixture isolation, Ginkgo scheduling metadata and full discovery.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
command -v jq >/dev/null || { echo 'jq is required to verify addon/module E2E discovery' >&2; exit 1; }
report_dir=$(mktemp -d "${TMPDIR:-/tmp}/kubevela-addon-module-discovery.XXXXXX")
trap 'rm -f "$report_dir"/*.json; rmdir "$report_dir"' EXIT

unit_tests='^Test(Scenario|Scoped|AddonModuleScheduling|AddonModuleWorker)'
# go test -run succeeds when nothing matches, which would turn this check into
# a no-op after a rename.
listed=$(go test ./test/e2e-addon-module-test -list "$unit_tests")
if ! grep -q '^Test' <<<"$listed"; then
  echo "no addon/module scheduling or scope tests match $unit_tests" >&2
  exit 1
fi
go test ./test/e2e-addon-module-test -run "$unit_tests" -count=1 -v
go test ./test/e2e-addon-module-test -run '^TestAddonModuleE2E$' -count=1 \
  -ginkgo.dry-run -ginkgo.fail-on-empty -ginkgo.json-report="$report_dir/all.json"

# Update when addon/module coverage changes intentionally.
expected_specs=119

jq -e -r --argjson expected "$expected_specs" '
  .[0].SpecReports | map(select(.LeafNodeType == "It")) as $specs |
  if ($specs | length) != $expected then
    error("addon/module discovery found \($specs | length) specs, want \($expected)")
  elif any($specs[]; .State != "passed") then error("addon/module discovery skipped specs")
  elif ($specs | map([.ContainerHierarchyTexts, .LeafNodeText]) | unique | length) != ($specs | length) then
    error("duplicate addon/module specs were discovered")
  else "Addon/module suite verified: \($specs | length) unique specs"
  end
' "$report_dir/all.json"

# make e2e-addon-module-test can split the suite across two clusters with this
# filter and its negation; together they must still run every spec once.
expected_serial_specs=13
serial_filter=$(<test/e2e-addon-module-test/serial-label-filter)
for part in serial rest; do
  filter=$serial_filter
  [[ "$part" == rest ]] && filter="!($serial_filter)"
  go test ./test/e2e-addon-module-test -run '^TestAddonModuleE2E$' -count=1 \
    -ginkgo.dry-run -ginkgo.label-filter="$filter" -ginkgo.json-report="$report_dir/$part.json" >/dev/null
done
jq -e -r -s --argjson expected "$expected_serial_specs" --argjson total "$expected_specs" '
  map(.[0].SpecReports | map(select(.LeafNodeType == "It" and .State == "passed") | [.ContainerHierarchyTexts, .LeafNodeText])) as [$serial, $rest] |
  if ($serial | length) != $expected then
    error("serial-label-filter selects \($serial | length) specs, want \($expected)")
  elif (($serial + $rest) | unique | length) != $total or ($serial | length) + ($rest | length) != $total then
    error("serial-label-filter and its negation select \($serial | length) + \($rest | length) specs, want \($total) distinct")
  else "Two-cluster split verified: \($serial | length) serial + \($rest | length) parallel specs"
  end
' "$report_dir/serial.json" "$report_dir/rest.json"
