#!/usr/bin/env bash
# Exercise real fixture isolation, Ginkgo scheduling metadata and full discovery.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
command -v jq >/dev/null || { echo 'jq is required to verify addon/module E2E discovery' >&2; exit 1; }
report_dir=$(mktemp -d "${TMPDIR:-/tmp}/kubevela-addon-module-discovery.XXXXXX")
trap 'rm -f "$report_dir"/all.json; rmdir "$report_dir"' EXIT

go test ./test/e2e-addon-module-test -run '^Test(Scenario|Scoped|AddonModuleScheduling|AddonModuleWorker)' -count=1 -v
go test ./test/e2e-addon-module-test -run '^TestAddonModuleE2E$' -count=1 \
  -ginkgo.dry-run -ginkgo.fail-on-empty -ginkgo.json-report="$report_dir/all.json"

# Update when addon/module coverage changes intentionally.
expected_specs=82

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
