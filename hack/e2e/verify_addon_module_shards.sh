#!/usr/bin/env bash
# Prove the isolated addon/module jobs select every spec exactly once.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
command -v jq >/dev/null || { echo 'jq is required to verify addon/module E2E discovery' >&2; exit 1; }
report_dir=$(mktemp -d "${TMPDIR:-/tmp}/kubevela-addon-module-discovery.XXXXXX")
trap 'rm -f "$report_dir"/*.json; rmdir "$report_dir"' EXIT
shards=(install versions recovery cache errors)

discover() {
  go test ./test/e2e-addon-module-test -run '^TestAddonModuleE2E$' -count=1 \
    -ginkgo.dry-run -ginkgo.fail-on-empty -ginkgo.label-filter="$2" -ginkgo.json-report="$report_dir/$1.json"
}

discover all ''
jq -e '
  def labels: ((.ContainerHierarchyLabels | flatten) + (.LeafNodeLabels // []))
    | map(select(startswith("addon-module-"))) | unique;
  .[0].SpecReports | map(select(.LeafNodeType == "It")) |
  if length == 0 then error("addon/module discovery found no specs")
  elif any(.[]; .State != "passed") then error("unfiltered addon/module discovery did not run every spec")
  elif any(.[]; (labels | length) != 1) then error("each addon/module spec must belong to exactly one isolated job")
  else true end
' "$report_dir/all.json"

for shard in "${shards[@]}"; do
  discover "$shard" "addon-module-$shard"
done

jq -e -r -s '
  def specs: .[0].SpecReports | map(select(.LeafNodeType == "It"));
  def identity: [.ContainerHierarchyTexts, .LeafNodeText];
  def labels: ((.ContainerHierarchyLabels | flatten) + (.LeafNodeLabels // []))
    | map(select(startswith("addon-module-"))) | unique;
  (.[0] | specs) as $baseline |
  .[1:] as $reports |
  ["addon-module-install", "addon-module-versions", "addon-module-recovery", "addon-module-cache", "addon-module-errors"] as $shards |
  ($reports | map(specs | map(select(.State == "passed"))) | add) as $selected |
  ($reports[1] | specs | map(select(.State == "passed")) | to_entries) as $versions |
  ($versions | map(select(any(.value.ContainerHierarchyTexts[]; endswith("(scenario 08)")))) | map(.key)) as $latest |
  ($versions | map(select(any(.value.ContainerHierarchyTexts[]; endswith("(scenario 07)")))) | map(.key)) as $upgrade |
  if any(range(0; $shards | length); . as $i |
      ($reports[$i] | specs | map(select(.State == "passed")) | length) == 0 or
      ($reports[$i] | specs | any(.[]; (.State == "passed") != (labels == [$shards[$i]])))) then
    error("an addon/module job is empty or selected the wrong specs")
  elif ($selected | map(identity) | sort) != ($baseline | map(identity) | sort) then
    error("addon/module jobs omit or repeat specs")
  elif ($selected | map(identity) | unique | length) != ($selected | length) then
    error("duplicate addon/module specs were discovered")
  elif ($latest | length) == 0 or ($upgrade | length) == 0 or ($latest | max) >= ($upgrade | min) then
    error("scenario 08 must publish versions before scenario 07 in the same job")
  else "Addon/module jobs verified: \($selected | length) unique specs",
    (range(0; $shards | length) | . as $i |
      "\($shards[$i]): \($reports[$i] | specs | map(select(.State == "passed")) | length) specs")
  end
' "$report_dir/all.json" "$report_dir/install.json" "$report_dir/versions.json" \
  "$report_dir/recovery.json" "$report_dir/cache.json" "$report_dir/errors.json"
