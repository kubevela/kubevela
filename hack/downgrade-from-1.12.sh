#!/usr/bin/env bash
# Downgrades a vela-core release from 1.12 to a 1.11 chart.
#
# helm rollback cannot do it: 1.12 no longer ships the builtin definitions in the release
# manifest (a hook Job applies them). How they get their 1.11 content back depends on the target:
#   1.11.1 and later apply them from the same kind of hook Job, which runs during the downgrade;
#   1.11.0 renders them into the release, so this script first re-applies them with their 1.11
#   content and hands them to Helm, which a plain helm upgrade would refuse to adopt.
# Either way it downgrades and removes the admission webhooks only 1.12 serves.
#
# Usage:
#   hack/downgrade-from-1.12.sh [-n] [--release NAME] [--namespace NS] [--version 1.11.x] [--chart REF]
#     -n, --dry-run   show what would change; change nothing
#     --release       Helm release name (default kubevela)
#     --namespace     release namespace (default vela-system)
#     --version       1.11 chart version to downgrade to (default 1.11.2, the first that keeps pinned definition
#                     revisions)
#     --chart         chart reference (default kubevela/vela-core; a local path works too, if its
#                     Chart.yaml version is --version or matches a 1.11.x wildcard)
#     --image-repository  controller image repository, when a 1.12 value pinned another one
#
# The controller image tag is set to v<version>, whatever the release's values pinned: a tag
# kept from 1.12 would leave the 1.12 controller running on the 1.11 chart. A repository
# override (a mirror) is kept unless --image-repository replaces it.
#
# Keep definitionRevisionLimit at least as high as it is now: the 1.11 controller garbage
# collects revisions over the limit, pinned ones included, unless it carries the pin fix.
set -euo pipefail

release=kubevela
namespace=vela-system
version=1.11.2
chart=kubevela/vela-core
image_repository=""
dry_run=false

while [ $# -gt 0 ]; do
	case "$1" in
	-n | --dry-run) dry_run=true ;;
	--release) release="$2"; shift ;;
	--namespace) namespace="$2"; shift ;;
	--version) version="$2"; shift ;;
	--chart) chart="$2"; shift ;;
	--image-repository) image_repository="$2"; shift ;;
	-h | --help) sed -n '2,27p' "$0"; exit 0 ;;
	*) echo "error: unknown argument $1" >&2; exit 2 ;;
	esac
	shift
done

# Webhooks the 1.12 chart registers for kinds 1.11 does not serve. Left behind, they refuse
# every write of those kinds, since nothing answers them.
only_112_webhooks="validating.config.oam.dev.v1alpha1.configtemplates validating.config.oam.dev.v1alpha1.configs validating.config.oam.dev.v1alpha1.configsecrets validating.core.oam-dev.v1beta1.sourcedefinitions"

# run makes one change, or in a dry run says what it would be. A command's own output is not
# shown; its errors are.
run() {
	if $dry_run; then
		echo "would: $*"
	else
		"$@" >/dev/null
	fi
}

for tool in helm kubectl; do
	command -v "$tool" >/dev/null || { echo "error: $tool is not installed" >&2; exit 1; }
done
case "$version" in
1.11.*) ;;
*) echo "error: --version must be a 1.11 chart version, got $version" >&2; exit 1 ;;
esac
if ! chart_meta="$(helm show chart "$chart" --version "$version" 2>/dev/null)"; then
	echo "error: chart $chart version $version not found (helm repo add kubevela https://kubevela.github.io/charts && helm repo update, if the repo is missing)" >&2
	exit 1
fi
chart_version="$(echo "$chart_meta" | sed -n 's/^version: *//p' | tr -d "\"'")"
# Helm ignores --version for a local chart path, so the chart's own version must satisfy it: equal to
# an exact version, or matching a 1.11.x wildcard. A repo chart is the one Helm resolved it to.
if [ -e "$chart" ]; then
	case "$version" in
	*[!0-9.xX*]*) echo "error: a local chart path needs --version as a 1.11 version or a 1.11.x wildcard, got $version" >&2; exit 1 ;;
	esac
	pattern="$(echo "$version" | sed 's/[xX*]/*/g')"
	# shellcheck disable=SC2254 # the pattern is meant to glob
	case "$chart_version" in
	$pattern) ;;
	*) echo "error: chart $chart is version $chart_version, not $version (a local chart path is used as it is, whatever --version says)" >&2; exit 1 ;;
	esac
fi
# The resolved version from here on: the upgrade installs exactly the chart checked above, and the
# controller image tag names a real release.
version="$chart_version"
latest="$(helm history "$release" -n "$namespace" --max 1 -o json 2>/dev/null || true)"
current="$(echo "$latest" | sed -n 's/.*"chart":"vela-core-\([^"]*\)".*/\1/p')"
status="$(echo "$latest" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')"
[ -n "$current" ] || { echo "error: no vela-core release $release in namespace $namespace" >&2; exit 1; }
# A failed earlier attempt leaves the release at the 1.11 chart, not deployed: run again.
case "$current/$status" in
1.11.*/deployed | 1.10.*/deployed | 1.9.*/deployed) echo "error: release $release is already at chart $current" >&2; exit 1 ;;
esac

# The 1.11 chart is applied the way it was written for: Helm 4 applies server side by default,
# which conflicts with the fields other managers own (webhook-certgen's caBundle and
# failurePolicy), so it is told not to.
helm_apply=""
case "$(helm version --short)" in
v3.*) ;;
*) helm_apply="--server-side=false" ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# The 1.11 builtins, rendered with this release's values. A chart that applies them from a hook
# Job (1.11.1+) re-applies them itself during the downgrade; one that renders them into the
# release (1.11.0) needs them back under Helm first.
helm get values "$release" -n "$namespace" -o yaml >"$tmp/values.yaml"
helm template "$release" "$chart" --version "$version" -n "$namespace" -f "$tmp/values.yaml" \
	--output-dir "$tmp/rendered" >/dev/null
builtins=""
if [ -n "$(find "$tmp/rendered" -path '*/templates/builtin-definitions/*' -name '*.yaml')" ]; then
	hook_job=true
else
	hook_job=false
	builtins="$(find "$tmp/rendered" -path '*/templates/defwithtemplate/*' -name '*.yaml' | sort)"
	[ -n "$builtins" ] || { echo "error: the $version chart rendered no builtin definitions" >&2; exit 1; }
fi

# 1. Release-owned builtins only: put them back as 1.11 renders them, under Helm's field manager.
# Done before the upgrade: it leaves the 1.12 content of fields the 1.11 manifest does not change,
# and a Helm that applies server side would conflict with the 1.12 Job that applied them.
for f in $builtins; do
	run kubectl apply --server-side --force-conflicts --field-manager=helm -n "$namespace" -f "$f"
done

# 2. Release-owned builtins only: hand them to Helm, so the 1.11 release adopts rather than
# refuses them. After the apply above: its manager may be the one that owns these labels, and
# would drop them.
adopt=0
for f in $builtins; do
	for ref in $(kubectl apply --dry-run=client -f "$f" -o name 2>/dev/null); do
		kubectl get "$ref" -n "$namespace" >/dev/null 2>&1 || continue
		run kubectl label "$ref" -n "$namespace" --overwrite app.kubernetes.io/managed-by=Helm
		run kubectl annotate "$ref" -n "$namespace" --overwrite \
			meta.helm.sh/release-name="$release" meta.helm.sh/release-namespace="$namespace" \
			helm.sh/resource-policy-
		adopt=$((adopt + 1))
	done
done

# 3. Downgrade the release, keeping its values on the 1.11 chart's defaults. A hook Job chart
# re-applies its builtins here, and Helm fails the upgrade if that Job fails.
image_set="--set image.tag=v$version"
[ -n "$image_repository" ] && image_set="$image_set --set image.repository=$image_repository"
run helm upgrade "$release" "$chart" --version "$version" -n "$namespace" --reset-then-reuse-values $image_set $helm_apply --wait --timeout 10m

# 4. Remove the webhooks only 1.12 serves.
removed=0
for vwc in $(kubectl get validatingwebhookconfigurations -o name); do
	names="$(kubectl get "$vwc" -o jsonpath='{range .webhooks[*]}{.name}{"\n"}{end}')"
	i=0
	indices=""
	for name in $names; do
		for stale in $only_112_webhooks; do
			[ "$name" = "$stale" ] && indices="$i $indices"
		done
		i=$((i + 1))
	done
	for idx in $indices; do
		run kubectl patch "$vwc" --type json -p "[{\"op\":\"remove\",\"path\":\"/webhooks/$idx\"}]"
		removed=$((removed + 1))
	done
done

# 5. Verify: nothing left to change in release-owned builtins, no 1.12-only webhook registered,
# and the controller on the 1.11 image.
if ! $dry_run; then
	drift=0
	for f in $builtins; do
		kubectl diff --server-side --force-conflicts --field-manager=helm -n "$namespace" -f "$f" >/dev/null 2>&1 || drift=$((drift + 1))
	done
	left="$(kubectl get validatingwebhookconfigurations -o jsonpath='{range .items[*].webhooks[*]}{.name}{"\n"}{end}' | grep -cE "$(echo "$only_112_webhooks" | tr ' ' '|')" || true)"
	image="$(kubectl get deploy "$release-vela-core" -n "$namespace" -o jsonpath='{.spec.template.spec.containers[0].image}')"
	if [ "$drift" -ne 0 ] || [ "$left" -ne 0 ] || [ "${image##*:}" != "v$version" ]; then
		echo "error: after the downgrade $drift builtin files still differ from $version, $left 1.12-only webhooks remain, and the controller runs $image" >&2
		exit 1
	fi
fi

mode=""
$dry_run && mode=" (dry run)"
if $hook_job; then
	applied="builtins re-applied by the $version chart's hook Job"
else
	applied="$adopt builtins handed to Helm, $(echo "$builtins" | wc -l | tr -d ' ') builtin files re-applied"
fi
echo "summary$mode: $release $current -> $version, $applied, $removed webhooks removed"
