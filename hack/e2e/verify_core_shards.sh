#!/usr/bin/env bash
# Verify the physical core suite packages and their classifications.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
command -v jq >/dev/null || { echo 'jq is required to verify core E2E discovery' >&2; exit 1; }
report_dir=$(mktemp -d "${TMPDIR:-/tmp}/kubevela-core-discovery.XXXXXX")
trap 'rm -f "$report_dir"/*.json; rmdir "$report_dir"' EXIT

go test ./test/e2e-framework -count=1
for suite in application definition config helm helm-auth; do
  go test "./test/e2e-${suite}-test" -run '^TestAPIs$' -count=1 -ginkgo.dry-run \
    -ginkgo.json-report="$report_dir/$suite.json"
done

jq -e -r -s '
  def specs: .[0].SpecReports | map(select(.LeafNodeType == "It" and .State != "pending"));
  def identity: [.ContainerHierarchyTexts, .LeafNodeText];
  def groups: {
    "e2e-application-test": "core-application",
    "e2e-definition-test": "core-definitions",
    "e2e-config-test": "core-config",
    "e2e-helm-test": "core-helm",
    "e2e-helm-auth-test": "core-helm-auth"
  };
  def group: .[0].SuitePath | split("/")[-1] | groups[.];
  def core_labels: ((.ContainerHierarchyLabels | flatten) + (.LeafNodeLabels // []))
    | map(select(startswith("core-"))) | unique;
  (map(specs) | add) as $all |
  if any(.[]; (specs | length) == 0) then error("a core suite discovered no specs")
  elif any($all[]; .State != "passed") then error("core discovery unexpectedly skipped specs")
  elif any(.[]; group as $group | any(specs[]; core_labels != [$group])) then
    error("a core test is missing its classification or is in the wrong suite folder")
  elif ($all | map(identity) | unique | length) != ($all | length) then
    error("duplicate core specs were discovered in the suite packages")
  else "Core packages verified: \($all | length) unique specs",
    (.[] | group as $group | "\($group): \(specs | length) specs")
  end
' "$report_dir/application.json" "$report_dir/definition.json" \
  "$report_dir/config.json" "$report_dir/helm.json" "$report_dir/helm-auth.json"
