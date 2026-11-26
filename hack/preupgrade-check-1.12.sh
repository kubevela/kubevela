#!/usr/bin/env bash
# Checks a running KubeVela 1.11 cluster for what the 1.12 upgrade refuses or changes, and prints a
# fix for each finding. It only reads: kubectl get and list, helm get values and helm list.
#
# Usage:
#   hack/preupgrade-check-1.12.sh [--release NAME] [--namespace NS] [--json] [-v | -q] [--resolve] [--from-dir DIR]
#     --release     Helm release name (default kubevela)
#     --namespace   release namespace (default vela-system)
#     --json        print the findings as JSON
#     -v            also list each check with its finding count
#     -q            print only the summary line
#     --resolve     resolve the hostnames workflow steps call, from this machine (not from the
#                   cluster, whose DNS can differ), and check the addresses too
#     --from-dir    read captured files from DIR instead of the cluster (see collect below for names)
#
# Exit codes: 0 no errors, 1 errors found, 2 the check could not run (a missing tool, no cluster,
# KubeVela not installed).
#
# Needs kubectl and jq. helm is optional: without it the release values are not read, and the
# checks that depend on them assume the gates the controller runs with now and the 1.12 defaults.
#
# Findings are judged for the controller version running now. A change that was backported to the
# running version already applies, so an object it refuses is reported as a warning that it fails
# today, with the same fix; before the backport it is an error the upgrade introduces.
set -euo pipefail

release=kubevela
namespace=vela-system
format=text
verbosity=normal
from_dir=""
resolve=false

while [ $# -gt 0 ]; do
	case "$1" in
	--release) release="$2"; shift ;;
	--namespace) namespace="$2"; shift ;;
	--json) format=json ;;
	-v | --verbose) verbosity=verbose ;;
	-q | --quiet) verbosity=quiet ;;
	--resolve) resolve=true ;;
	--from-dir) from_dir="$2"; shift ;;
	-h | --help) sed -n '2,24p' "$0"; exit 0 ;;
	*) echo "error: unknown argument $1" >&2; exit 2 ;;
	esac
	shift
done

command -v jq >/dev/null || { echo "error: jq is not installed" >&2; exit 2; }

tmp="$(mktemp -d)"
chmod 700 "$tmp"
trap 'rm -rf "$tmp"' EXIT

# Definitions shared by the collector's queries and the analysis.
read -r -d '' lib <<'JQ' || true
def items($f): ($f[0] // {}) | (.items // []);
def nn: "\(.metadata.namespace)/\(.metadata.name)";
def trim: gsub("^\\s+|\\s+$"; "");
def byname($l): reduce $l[] as $d ({}; .[$d | nn] = $d);

# The steps an Application runs: its own, or those of the Workflow it references (its namespace,
# then the system one), with sub-steps.
def appsteps($wfidx; $sysns):
  .metadata.namespace as $appns
  | (if ((.spec.workflow.steps // []) | length) > 0 then .spec.workflow.steps
     elif (.spec.workflow.ref // "") != "" then
       (($wfidx[$appns + "/" + .spec.workflow.ref] // $wfidx[$sysns + "/" + .spec.workflow.ref]).steps // [])
     else [] end)
  | .[] | ., (.subSteps // [])[];

# The URLs an HTTP step calls, with where each sits in its properties: a literal url, or the
# Secret key a webhook or notification reads it from.
def stepurls:
  .properties as $p
  | if .type == "request" then [{at: "url", url: $p.url}]
    elif .type == "webhook" then [{at: "url", url: $p.url.value?, secret: $p.url.secretRef?}]
    elif .type == "notification" then
      [("lark", "dingding", "slack") as $k | select($p[$k] != null)
        | {at: "\($k).url", url: $p[$k].url.value?, secret: $p[$k].url.secretRef?}]
    else [] end
  | map(select((.url | type == "string") or (.secret | type == "object")));

def urlhost:
  sub("^[A-Za-z][A-Za-z0-9+.-]*://"; "") | sub("[/?#].*$"; "") | sub("^.*@"; "")
  | if startswith("[") then (capture("^\\[(?<h>[^\\]]*)\\]").h // "") else sub(":[0-9]*$"; "") end
  | ascii_downcase | rtrimstr(".");
def isip: test("^[0-9]{1,3}(\\.[0-9]{1,3}){3}$") or (test(":") and test("^[0-9a-f:.]+$"));
JQ

# collect writes one file per kind into $tmp. Every list is read cluster wide. A kind the cluster
# does not serve is written as an empty list only where that is expected (Argo CD).
collect() {
	command -v kubectl >/dev/null || { echo "error: kubectl is not installed" >&2; exit 2; }
	if ! err="$(kubectl get --raw /version 2>&1 >/dev/null)"; then
		echo "error: cannot reach the cluster: $err" >&2
		exit 2
	fi
	if ! kubectl get crd applications.core.oam.dev -o json >"$tmp/crd-applications.json" 2>"$tmp/err"; then
		echo "error: KubeVela is not installed (no applications.core.oam.dev CRD): $(cat "$tmp/err")" >&2
		exit 2
	fi
	if command -v helm >/dev/null; then
		if ! helm get values "$release" -n "$namespace" -o json >"$tmp/helm-values.json" 2>"$tmp/err"; then
			echo "error: cannot read the values of release $release in $namespace: $(cat "$tmp/err")" >&2
			exit 2
		fi
		helm list -n "$namespace" --filter "^${release}\$" -o json >"$tmp/helm-release.json"
	fi
	local kind file
	for kind in applications.core.oam.dev:applications workflows.core.oam.dev:workflows \
		componentdefinitions.core.oam.dev:componentdefinitions traitdefinitions.core.oam.dev:traitdefinitions \
		policydefinitions.core.oam.dev:policydefinitions workflowstepdefinitions.core.oam.dev:workflowstepdefinitions \
		definitionrevisions.core.oam.dev:definitionrevisions namespaces:namespaces; do
		file="${kind#*:}"
		if ! kubectl get "${kind%%:*}" -A -o json >"$tmp/$file.json" 2>"$tmp/err"; then
			echo "error: cannot list ${kind%%:*}: $(cat "$tmp/err")" >&2
			exit 2
		fi
	done
	if ! kubectl get configmaps -A -l config.oam.dev/catalog=velacore-config -o json >"$tmp/configmaps.json" 2>"$tmp/err"; then
		echo "error: cannot list config template ConfigMaps: $(cat "$tmp/err")" >&2
		exit 2
	fi
	# The HTTP guard's deny lists: ConfigMaps in the controller's namespace labelled with the
	# deny template's name.
	local deny_template
	deny_template="$(jq -r '.workflow.httpDeny.configTemplateName // "vela-core-http-deny"' "$tmp/helm-values.json" 2>/dev/null || echo vela-core-http-deny)"
	if ! kubectl get configmaps -n "$namespace" -l "config.oam.dev/type=$deny_template" -o json >"$tmp/deny-configmaps.json" 2>"$tmp/err"; then
		echo "error: cannot list the HTTP deny ConfigMaps: $(cat "$tmp/err")" >&2
		exit 2
	fi
	# Config Secrets keep their metadata and the objects-reference key only.
	if ! kubectl get secrets -A -l config.oam.dev/catalog=velacore-config -o json 2>"$tmp/err" |
		jq '.items |= map(.data = {"objects-reference": (.data["objects-reference"] // null)})' >"$tmp/secrets.json"; then
		echo "error: cannot list config Secrets: $(cat "$tmp/err")" >&2
		exit 2
	fi
	# The release's cluster-gateway Deployment carries the same name label, so the controller's is
	# told apart by its own.
	if ! kubectl get deployments -n "$namespace" -l "app.kubernetes.io/instance=$release,controller.oam.dev/name=vela-core" \
		-o json >"$tmp/deployment.json" 2>"$tmp/err"; then
		echo "error: cannot read the vela-core Deployment: $(cat "$tmp/err")" >&2
		exit 2
	fi
	if [ "$(jq '.items | length' "$tmp/deployment.json")" = 0 ]; then
		echo "error: no vela-core Deployment of release $release in namespace $namespace (pass --release and --namespace)" >&2
		exit 2
	fi
	if kubectl get crd applications.argoproj.io >/dev/null 2>&1; then
		kubectl get applications.argoproj.io -A -o json >"$tmp/argocd-applications.json" 2>"$tmp/err" ||
			echo '{"items":[]}' >"$tmp/argocd-applications.json"
	fi
	collect_secret_hosts
	if $resolve; then
		echo "note: --resolve looks hostnames up from this machine, not from the cluster, whose DNS can answer differently" >&2
		collect_resolved
	fi
}

# steps_query lists, one per line, what jq program $1 derives from every step the Applications run.
steps_query() {
	jq -r -n --arg sysns "$namespace" --slurpfile apps "$tmp/applications.json" --slurpfile wfs "$tmp/workflows.json" \
		"$lib"'
		byname(items($wfs)) as $wfidx
		| items($apps)[] as $a | $a | appsteps($wfidx; $sysns) | '"$1"
}

# collect_secret_hosts reads each Secret key a webhook or notification step takes its URL from, and
# keeps only the URL's host. The value itself is never written or printed. A Secret that cannot be
# read is recorded with the reason, and its URL left unchecked.
collect_secret_hosts() {
	local ns name key host err
	echo '{"items":[' >"$tmp/secret-hosts.json"
	local sep=""
	steps_query 'stepurls[] | select(.secret.name != null) | [$a.metadata.namespace, .secret.name, (.secret.key // "")] | @tsv' |
		sort -u >"$tmp/secret-refs.tsv"
	while IFS="$(printf '\t')" read -r ns name key; do
		[ -n "$name" ] || continue
		if host="$(kubectl get secret -n "$ns" "$name" -o go-template="{{index .data \"$key\"}}" 2>"$tmp/err" |
			jq -Rr "$lib"' @base64d | urlhost' 2>/dev/null)"; then
			printf '%s{"namespace":"%s","name":"%s","key":"%s","host":%s}\n' "$sep" "$ns" "$name" "$key" \
				"$(printf '%s' "$host" | jq -Rs .)" >>"$tmp/secret-hosts.json"
		else
			err="$(head -n 1 "$tmp/err")"
			printf '%s{"namespace":"%s","name":"%s","key":"%s","skipped":%s}\n' "$sep" "$ns" "$name" "$key" \
				"$(printf '%s' "${err:-no such key}" | jq -Rs .)" >>"$tmp/secret-hosts.json"
		fi
		sep=","
	done <"$tmp/secret-refs.tsv"
	echo ']}' >>"$tmp/secret-hosts.json"
}

# resolve_host prints the addresses this machine resolves a hostname to, one per line.
resolve_host() {
	if command -v getent >/dev/null; then
		getent ahosts "$1" 2>/dev/null | awk '{print $1}'
	elif command -v dscacheutil >/dev/null; then
		dscacheutil -q host -a name "$1" 2>/dev/null | awk '/^ip(v6)?_address:/ {print $2}'
	elif command -v dig >/dev/null; then
		dig +short A "$1" 2>/dev/null
		dig +short AAAA "$1" 2>/dev/null
	fi
}

collect_resolved() {
	local host
	echo '{}' >"$tmp/resolved.json"
	for host in $(steps_query 'stepurls[] | select(.url != null) | .url | urlhost | select(isip | not)' | sort -u); do
		resolve_host "$host" | { grep -E '^[0-9a-fA-F:.]+$' || true; } | sort -u | jq -R . | jq -s --arg h "$host" '{($h): .}' >"$tmp/one.json"
		jq -s '.[0] * .[1]' "$tmp/resolved.json" "$tmp/one.json" >"$tmp/merged.json"
		mv "$tmp/merged.json" "$tmp/resolved.json"
	done
}

if [ -n "$from_dir" ]; then
	[ -d "$from_dir" ] || { echo "error: --from-dir $from_dir is not a directory" >&2; exit 2; }
	[ -f "$from_dir/crd-applications.json" ] || { echo "error: $from_dir has no crd-applications.json" >&2; exit 2; }
	cp "$from_dir"/*.json "$tmp/"
else
	collect
fi

# Lists that are absent are empty; the helm files stay absent, which the analysis reports.
for f in applications workflows componentdefinitions traitdefinitions policydefinitions workflowstepdefinitions \
	definitionrevisions namespaces configmaps deny-configmaps secrets secret-hosts deployment argocd-applications; do
	[ -s "$tmp/$f.json" ] || echo '{"items":[]}' >"$tmp/$f.json"
done
[ -s "$tmp/resolved.json" ] || echo '{}' >"$tmp/resolved.json"
helm_read=true
if [ ! -f "$tmp/helm-values.json" ]; then
	helm_read=false
	echo '{}' >"$tmp/helm-values.json"
	echo '[]' >"$tmp/helm-release.json"
fi
[ -f "$tmp/helm-release.json" ] || echo '[]' >"$tmp/helm-release.json"

# The analysis: one finding per object and problem, as {level, check, object, problem, fix, link}.
read -r -d '' analysis <<'JQ' || true
def guide($a): "https://kubevela.io/upgrade-guides/v1.12/#" + $a;
def notes: "https://kubevela.io/release-notes/v1.12/v1.12.0/#breaking-changes";
def f($level; $check; $object; $problem; $fix; $link):
  {level: $level, check: $check, object: $object, problem: $problem, fix: $fix, link: $link};

# A version as [major, minor, patch, release], a pre-release sorting before its release.
def semver:
  (capture("(?<a>[0-9]+)\\.(?<b>[0-9]+)\\.(?<c>[0-9]+)(?<pre>-[0-9A-Za-z.-]+)?") // null)
  | if . == null then null else [(.a | tonumber), (.b | tonumber), (.c | tonumber), (if .pre then 0 else 1 end)] end;

# The release each change shipped in, from the tags: v1.11.1 carries the trait conflict webhook and
# the HTTP guard's deny lists; v1.11.2 the loopback denial, the timeout bounds, the Application
# namespace rule for helmchart sources, Terraform SSH keys and config steps, and the vela-cli Job's
# account.
def first_version: {
  "trait-conflicts": "1.11.1", "http-deny": "1.11.1", "http-loopback": "1.11.2", "http-timeout": "1.11.2",
  "helm-sources": "1.11.2", "terraform-ssh": "1.11.2", "vela-cli-account": "1.11.2", "config-steps": "1.11.2"};

($values[0] // {}) as $vals
| ($vals.systemDefinitionNamespace // $ns | if . == "" or . == null then $ns else . end) as $sysns
| (items($deploy)[0].spec.template.spec.containers[0] // {}) as $ctr
| ($ctr.args // []) as $args
| ([$args[] | select(startswith("--feature-gates=")) | ltrimstr("--feature-gates=") | split(",")[]
    | select(contains("=")) | split("=") | {(.[0]): (.[1] == "true")}] | add // {}) as $now
| def arg($n): [$args[] | select(startswith("--" + $n + "=")) | ltrimstr("--" + $n + "=")] | last;

# The controller version running now: its image tag, else its version label, else the chart's.
  ([($ctr.image // "" | sub("@.*$"; "") | if test(":") then split(":") | last else "" end),
    (items($deploy)[0].metadata.labels["app.kubernetes.io/version"] // ""),
    ($release[0][0].chart // "" | ltrimstr("vela-core-"))]
   | map(select(semver != null)) | first // null) as $runningStr
| ($runningStr | if . == null then null else semver end) as $running
| def since($change): $running != null and $running >= (first_version[$change] | semver);
  def vlabel: if $runningStr == null then "an unknown version" else "v" + ($runningStr | ltrimstr("v")) end;

# A finding whose change may already apply: reported as failing today once the running version
# carries it, otherwise as what 1.12 refuses.
  def fx($change; $level; $check; $object; $detail; $refusal; $fix; $link):
    if since($change) then
      f("warn"; $check; $object; "already refused on \(vlabel) (since \(first_version[$change])): \($detail)"; $fix; $link)
    else f($level; $check; $object; "\($detail); \($refusal)"; $fix; $link) end;

# What the upgrade will run with: the release's values over the 1.12 defaults. Without the values,
# the gates the controller runs with now.
  def gate($value; $gate; $default):
    if $helmRead then (if $value == null then $default else $value end)
    else (if $now[$gate] == null then $default else $now[$gate] end) end;
  {BlockPrivateHTTPAddresses: gate($vals.featureGates.blockPrivateHTTPAddresses; "BlockPrivateHTTPAddresses"; false),
   DisableWorkflowHTTP: gate($vals.featureGates.disableWorkflowHTTP; "DisableWorkflowHTTP"; false),
   RestrictDefinitionNamespaces: gate($vals.featureGates.restrictDefinitionNamespaces; "RestrictDefinitionNamespaces"; true),
   AuthenticateApplication: gate($vals.authentication.enabled; "AuthenticateApplication"; false),
   EnableGlobalPolicies: gate($vals.featureGates.enableGlobalPolicies; "EnableGlobalPolicies"; false),
   EnableApplicationScopedPolicies: gate($vals.featureGates.enableApplicationScopedPolicies; "EnableApplicationScopedPolicies"; false),
   allowNamespaceGlobalPolicies: ($vals.controllerArgs.allowNamespaceGlobalPolicies // false),
   allowSkipGlobalPolicies: ($vals.controllerArgs.allowSkipGlobalPolicies // false)} as $after

# Go's time.ParseDuration, to seconds; null when it does not parse.
| def duration:
    if . == "0" or . == "+0" or . == "-0" then 0
    elif test("^[+-]?(([0-9]+(\\.[0-9]*)?|\\.[0-9]+)(ns|us|µs|μs|ms|s|m|h))+$") then
      (if startswith("-") then -1 else 1 end) as $sign
      | ([scan("([0-9]*\\.?[0-9]*)(ns|us|µs|μs|ms|s|m|h)") | (.[0] | tonumber) *
          ({"ns": 1e-9, "us": 1e-6, "µs": 1e-6, "μs": 1e-6, "ms": 1e-3, "s": 1, "m": 60, "h": 3600}[.[1]])] | add) * $sign
    else null end;
  (arg("max-workflow-http-timeout") // "60s") as $maxNowStr
| (if $helmRead then ($vals.workflow.http.maxTimeout // "60s") else $maxNowStr end) as $maxAfterStr
| ($maxNowStr | duration // 60) as $maxNow
| ($maxAfterStr | duration // 60) as $maxAfter

| byname(items($cdefs)) as $cidx | byname(items($tdefs)) as $tidx | byname(items($pdefs)) as $pidx
| byname(items($sdefs)) as $sidx | byname(items($wfs)) as $wfidx | byname(items($revs)) as $revidx
| def steps: appsteps($wfidx; $sysns);

# Definitions resolve as GetCapabilityDefinition does: the Application's namespace first, then the
# system one; type@vN reads the DefinitionRevision <type>-vN, falling back to the live definition.
  def resolve($kind; $type; $appns):
    ({"c": $cidx, "t": $tidx, "p": $pidx, "s": $sidx}[$kind]) as $idx
    | ($type | split("@")) as $parts
    | ($idx[$appns + "/" + $parts[0]] // $idx[$sysns + "/" + $parts[0]]) as $live
    | if ($parts | length) > 1 then
        ($parts[0] + "-" + $parts[1]) as $rn
        | ($revidx[$appns + "/" + $rn] // $revidx[$sysns + "/" + $rn]) as $rev
        | ({"c": "componentDefinition", "t": "traitDefinition", "p": "policyDefinition", "s": "workflowStepDefinition"}[$kind]) as $field
        | if $rev != null and $rev.spec[$field] != null then ($rev.spec[$field] + {_revision: $rev.metadata.name}) else $live end
      else $live end;

# A Kubernetes label selector (labels.Parse) against a label map. A term this does not understand
# matches nothing, as an unparseable selector does.
  def termmatch($l):
    if test("^!\\s*[A-Za-z0-9./_-]+$") then ($l | has(ltrimstr("!") | trim) | not)
    elif test("^\\S+\\s+notin\\s*\\(") then
      capture("^(?<k>\\S+)\\s+notin\\s*\\((?<v>[^)]*)\\)$") as $m
      | ($m.v | split(",") | map(trim)) as $vs
      | (($l | has($m.k) | not) or (($vs | index($l[$m.k])) == null))
    elif test("^\\S+\\s+in\\s*\\(") then
      capture("^(?<k>\\S+)\\s+in\\s*\\((?<v>[^)]*)\\)$") as $m
      | ($m.v | split(",") | map(trim)) as $vs
      | (($l | has($m.k)) and (($vs | index($l[$m.k])) != null))
    elif test("^[A-Za-z0-9./_-]+\\s*[<>]\\s*-?[0-9]+$") then
      capture("^(?<k>[A-Za-z0-9./_-]+)\\s*(?<op>[<>])\\s*(?<n>-?[0-9]+)$") as $m
      | ($l[$m.k] // "") as $v
      | ($v | test("^-?[0-9]+$")) and
        (if $m.op == ">" then ($v | tonumber) > ($m.n | tonumber) else ($v | tonumber) < ($m.n | tonumber) end)
    elif test("!=") then (split("!=") | map(trim)) as [$k, $v] | ($l[$k] != $v)
    elif test("=") then (split("=") | map(select(. != "")) | map(trim)) as [$k, $v] | ($l[$k] == $v)
    elif test("^[A-Za-z0-9./_-]+$") then ($l | has(.))
    else false end;
  def selmatch($expr; $l):
    ($expr | trim) as $e
    | if $e == "" then true
      else [$e | scan("(?:[^,(]|\\([^)]*\\))+") | trim | select(. != "")] | all(termmatch($l)) end;
# A metav1.LabelSelector (matchLabels, matchExpressions) against a label map.
  def lsmatch($sel; $l):
    all(($sel.matchLabels // {}) | to_entries[]; $l[.key] == .value)
    and all(($sel.matchExpressions // [])[];
      . as $e | ($l | has($e.key)) as $has
      | if $e.operator == "In" then $has and (($e.values // []) | index($l[$e.key])) != null
        elif $e.operator == "NotIn" then ($has | not) or (($e.values // []) | index($l[$e.key])) == null
        elif $e.operator == "Exists" then $has
        elif $e.operator == "DoesNotExist" then ($has | not)
        else false end);

# traitConflictRuleMatches in pkg/webhook/core.oam.dev/v1beta1/application/validation.go.
  def rulematch($rule; $t):
    ($t.spec.definitionRef.name // "") as $ref
    | if $rule == "*" then true
      elif ($rule | startswith("labelSelector:")) then selmatch($rule | ltrimstr("labelSelector:"); $t.metadata.labels // {})
      elif $rule == $t.metadata.name then true
      elif $ref != "" and $rule == $ref then true
      elif ($rule | startswith("*.")) then ($ref | index(".")) as $i | ($i != null and $ref[$i + 1:] == ($rule | ltrimstr("*.")))
      else false end;
  def conflicts($a; $b): any(($a.spec.conflictsWith // [])[]; rulematch(.; $b));

# path.Match, as definition restrictions and namespace patterns use it.
  def globmatch($p): test("^" + ($p | gsub("(?<c>[.+$(){}|])"; "\\\(.c)") | gsub("\\*"; "[^/]*") | gsub("\\?"; "[^/]")) + "$");

# Addresses as bit strings, for CIDR matching.
  def hex2n: explode | map(if . >= 97 then . - 87 elif . >= 65 then . - 55 else . - 48 end) | reduce .[] as $d (0; . * 16 + $d);
  def bits($w): . as $n | [range($w - 1; -1; -1) as $i | (($n / pow(2; $i)) | floor) % 2 | tostring] | join("");
  def v6groups:
    (if test("::") then (split("::") as [$a, $b]
        | ($a | if . == "" then [] else split(":") end) as $x
        | ($b | if . == "" then [] else split(":") end) as $y
        | $x + [range(8 - ($x | length) - ($y | length)) | "0"] + $y)
     else split(":") end) | map(("0000" + .)[-4:] | ascii_downcase);
  def ipbits:
    if test("^[0-9]{1,3}(\\.[0-9]{1,3}){3}$") then {fam: 4, bits: (split(".") | map(tonumber | bits(8)) | join(""))}
    elif test("^::ffff:[0-9]{1,3}(\\.[0-9]{1,3}){3}$") then ltrimstr("::ffff:") | ipbits
    elif test(":") and test("^[0-9a-f:]+$") then
      v6groups as $g
      | if ($g | length) == 8 and all($g[]; test("^[0-9a-f]{4}$")) then {fam: 6, bits: ($g | map(hex2n | bits(16)) | join(""))} else null end
    else null end;
  def cidrof: (split("/") as [$ip, $len] | ($ip | ascii_downcase | ipbits) as $b
    | if $b == null then null else {fam: $b.fam, prefix: $b.bits[0:(if $len then ($len | tonumber) else ($b.bits | length) end)]} end);
  def nat64: if .fam == 6 and (.bits | startswith("0000000001100100111111111001101100000000000000000000000000000000000000000000000000000000"[0:96]))
    then {fam: 4, bits: .bits[96:]} else null end;

# The HTTP guard's deny entries (kubevela/workflow httpguard, cmd/core/app/http_policy.go), each
# with whether it applies now and after the upgrade.
  ("http-deny" | since(.)) as $guardNow
| ("http-loopback" | since(.)) as $loopNow
| [ ( (["169.254.0.0/16", "a link-local address (169.254.0.0/16, cloud metadata)"], ["fe80::/10", "a link-local address (fe80::/10)"],
     ["fd00:ec2::254", "a cloud metadata address (fd00:ec2::254)"], ["100.100.100.200", "a cloud metadata address (100.100.100.200)"])
    | {cidr: (.[0] | cidrof), why: .[1], now: $guardNow, after: true} ),
    ( (["127.0.0.0/8", "a loopback address (127.0.0.0/8)"], ["::1", "the loopback address (::1)"],
     ["0.0.0.0/8", "an unspecified address (0.0.0.0/8)"], ["::", "the unspecified address (::)"],
     ["64:ff9b:1::/48", "a local-use NAT64 address (64:ff9b:1::/48)"])
    | {cidr: (.[0] | cidrof), why: .[1], now: $loopNow, after: true} ),
    (["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"][]
    | {cidr: cidrof, why: "a private address (\(.), refused with blockPrivateHTTPAddresses)",
       now: ($guardNow and ($now.BlockPrivateHTTPAddresses // false)), after: $after.BlockPrivateHTTPAddresses}),
    {host: "metadata.google.internal", why: "a cloud metadata host (metadata.google.internal)", now: $guardNow, after: true},
    ( items($deny)[] as $cm | ($cm.data.denyCIDRs // "", $cm.data.denyHosts // "") as $text
      | $text | split("\n")[] | sub("#.*$"; "") | trim | select(. != "") | ascii_downcase | rtrimstr(".")
      | (if (cidrof) != null then {cidr: cidrof} elif startswith("*.") then {suffix: ltrimstr("*.")} else {host: .} end)
        + {why: "in the deny list ConfigMap \($cm | nn)", now: $guardNow, after: true} ),
    ( select($helmRead and ($vals.workflow.httpDeny.defaultConfig.enabled // true))
      | ($vals.workflow.httpDeny.defaultConfig.denyCIDRs // [])[], ($vals.workflow.httpDeny.defaultConfig.denyHosts // [])[]
      | ascii_downcase | rtrimstr(".")
      | (if (cidrof) != null then {cidr: cidrof} elif startswith("*.") then {suffix: ltrimstr("*.")} else {host: .} end)
        + {why: "in workflow.httpDeny.defaultConfig in the values", now: false, after: true} ) ] as $denies
| def ipdenied($ip): ($ip | ipbits) as $b
    | if $b == null then [] else [$b, ($b | nat64) | select(. != null)] as $forms
      | [$denies[] | select(.cidr != null) | . as $d | select(any($forms[]; .fam == $d.cidr.fam and (.bits | startswith($d.cidr.prefix))))] end;
  def hostdenied($h):
    if $h | isip then ipdenied($h)
    else [$denies[] | select(.host == $h or (.suffix as $sfx | $sfx != null and $h != $sfx and ($h | endswith("." + $sfx))))]
      + (if $h == "localhost" or ($h | endswith(".localhost")) then ipdenied("127.0.0.1") | map(.why = "localhost, which resolves to " + .why) else [] end)
      + [($resolved[0][$h] // [])[] as $ip | ipdenied($ip)[] | .why = "\($h) resolves to \($ip) on this machine (resolved on this machine, not in the cluster), \(.why)"]
    end;

# Builtin definitions the 1.12 chart ships, by kind; it stamps definitionRestrictions on them.
  {"ComponentDefinition": "addon crate cron-task daemon helmchart k8s-objects raw ref-objects statefulset task webservice worker",
   "TraitDefinition": "affinity annotations command configmap container-image container-ports cpuscaler env expose gateway hostalias hpa ingress ingress-1-20 init-container json-merge-patch json-patch k8s-update-strategy labels lifecycle nocalhost node-affinity podsecuritycontext pure-ingress pvc resource scaler securitycontext service-account service-binding sidecar startup-probe storage topologyspreadconstraints volumes",
   "PolicyDefinition": "apply-once envbinding garbage-collect override read-only replication resource-update shared-resource take-over topology",
   "WorkflowStepDefinition": "apply-application apply-application-in-parallel apply-component apply-deployment apply-object apply-remaining apply-terraform-config apply-terraform-provider build-push-image check-metrics clean-jobs collect-service-endpoints create-config delete-config depends-on-app deploy deploy-cloud-resource deploy-components deploy2env deploy2runtime export-data export-service export2config export2secret generate-jdbc-connection list-config notification print-message-in-status read-config read-object request restart-workflow share-cloud-resource step-group suspend vela-cli webhook"}
  | map_values(split(" ")) as $builtins
| ({overrides: {addon: {namespaces: ["vela-system"]}, "vela-cli": {namespaces: ["vela-system"]}}} * (if $helmRead then ($vals.definitionRestrictions // {}) else {} end)) as $dr
| (reduce items($nss)[] as $n ({}; .[$n.metadata.name] = (($n.metadata.labels // {}) + {"kubernetes.io/metadata.name": $n.metadata.name}))) as $nslabels
# nsrestrict.Of: the restrictions in the spec (the chart stamps them on its builtins) when they
# restrict namespaces, else the definition.oam.dev/restrict-namespaces annotation.
| def restrictionsOf($kind; $d):
    (if $d._revision == null and $d.metadata.namespace == $sysns and (($builtins[$kind] // []) | index($d.metadata.name)) != null
     then ($dr.overrides[$d.metadata.name] // $dr.default // null) else null end) as $stamped
    | ([($d.metadata.annotations["definition.oam.dev/restrict-namespaces"] // "") | split(",")[] | trim | select(. != "")]) as $ann
    | if $stamped != null and ((($stamped.namespaces // []) | length) > 0 or $stamped.namespaceSelector != null) then $stamped
      elif ($ann | length) > 0 then {namespaces: $ann}
      else null end;
  def allows($r; $appns):
    $r == null
    or any(($r.namespaces // [])[]; . as $p | $appns | globmatch($p))
    or ($r.namespaceSelector != null and $nslabels[$appns] != null and lsmatch($r.namespaceSelector; $nslabels[$appns]));
  def describe($r):
    [(if (($r.namespaces // []) | length) > 0 then ($r.namespaces | join(", ")) else empty end),
     (if $r.namespaceSelector != null then "namespaces matching \($r.namespaceSelector | tojson)" else empty end)] | join(" or ");

  ($after.AuthenticateApplication) as $authn
| (reduce items($secrets)[] as $s ({}; .[$s | nn] = $s)) as $sidxs
| (reduce (items($cms)[] | select(.metadata.name | startswith("config-template-"))) as $c ({}; .[$c | nn] = $c)) as $tmpls
| (reduce items($shosts)[] as $h ({}; .["\($h.namespace)/\($h.name)/\($h.key)"] = $h)) as $hostOf
| def shared_config($name):
    ($sidxs["vela-system/" + $name].metadata.labels["config.oam.dev/type"] // "") as $t
    | $t != "" and ($tmpls["vela-system/config-template-" + $t] != null);
  def shared_template($template): ($template | split("/") | last) as $t | $tmpls["vela-system/config-template-" + $t] != null;

[
# 1. Trait conflicts.
  ( items($apps)[] as $a | $a.metadata.namespace as $appns
    | ($a.spec.components // [])[] as $c
    | [($c.traits // [])[] | resolve("t"; .type; $appns) | select(. != null)] as $defs
    | range(0; $defs | length) as $i | range($i + 1; $defs | length) as $j
    | select(conflicts($defs[$i]; $defs[$j]) or conflicts($defs[$j]; $defs[$i]))
    | fx("trait-conflicts"; "error"; "trait-conflicts"; "Application \($a | nn)";
        "component \($c.name): traits \($defs[$i].metadata.name) and \($defs[$j].metadata.name) conflict (conflictsWith), so every update is refused";
        "1.12 refuses the next spec change";
        "remove trait \($defs[$j].metadata.name) or \($defs[$i].metadata.name) from component \($c.name), or drop the rule from the TraitDefinition's spec.conflictsWith";
        guide("trait-conflicts")) ),

# 2. Helm values.
  ( if $helmRead then
      ($release[0][0].chart // "" | ltrimstr("vela-core-")) as $chart
      | f("info"; "helm-values"; "Release \($ns)/\($rel)";
          (if ($chart | test("^1\\.11\\.[01]$")) then "chart \($chart): helm upgrade --reuse-values fails to render the 1.12 chart with these values"
           else "chart \($chart): helm upgrade --reuse-values keeps the \($chart) image values, so vela-core and cluster-gateway are not upgraded" end);
          "upgrade with your own values file (-f), or --reset-then-reuse-values (Helm 3.14 and later), never --reuse-values";
          guide("config-templates-and-configs")),
      ( $vals.image.tag // empty | select(startswith("v1.12") | not)
        | f("error"; "helm-values"; "Release \($ns)/\($rel)";
            "the values pin image.tag \(.), so the upgrade keeps that controller image";
            "remove image.tag from your values file, or set it to the 1.12 tag you are installing";
            guide("config-templates-and-configs")) ),
      ( $vals.multicluster.clusterGateway.image.tag // empty | select(startswith("v1.10") | not)
        | f("error"; "helm-values"; "Release \($ns)/\($rel)";
            "the values pin multicluster.clusterGateway.image.tag \(.); 1.12 ships v1.10.0-alpha.1";
            "remove multicluster.clusterGateway.image.tag from your values file";
            notes) )
    else
      f("info"; "helm-values"; "Release \($ns)/\($rel)";
        "the release values were not read (no helm), so the checks that use them assume the running gates and the 1.12 defaults";
        "install helm and run again, or check the values yourself: no image.tag pin, and the global policy, restriction and HTTP settings";
        notes)
    end ),

# 3. CRDs.
  ( f("info"; "crds"; "CustomResourceDefinitions";
      "helm upgrade does not update the chart's crds/, and 1.12 adds the Config, ConfigTemplate and SourceDefinition CRDs";
      "before helm upgrade: helm pull kubevela/vela-core --version <1.12 version> --untar && kubectl apply --server-side --force-conflicts -f vela-core/crds/";
      guide("config-templates-and-configs")),
    ( [items($crd)[0] // $crd[0] | .spec.versions[]? | select(.name == "v1beta1")
        | .schema.openAPIV3Schema.properties.status.properties.appliedApplicationPolicies] | first
      | select(. == null)
      | f("info"; "crds"; "CustomResourceDefinition applications.core.oam.dev";
          "the Application CRD is older than 1.11's (no status.appliedApplicationPolicies): an earlier upgrade skipped the CRDs";
          "apply the 1.12 CRDs as above before helm upgrade; it brings the 1.11 fields too";
          guide("config-templates-and-configs")) ) ),

# 4. Credentials in other namespaces.
  ( items($apps)[] as $a | $a.metadata.namespace as $appns
    | ([($a.spec.policies // [])[] | select(.type == "topology") | .properties.namespace // empty | select(. != $appns)] | first) as $topons
    | ($a.spec.components // [])[] | select(.type == "helmchart") as $c | ($c.properties // {}) as $p
    | ( ($p.valuesFrom // [])[] | select((.namespace // "") != "" and .namespace != $appns)
        | fx("helm-sources"; "error"; "cross-namespace-credentials"; "Application \($a | nn)";
            "component \($c.name): valuesFrom \(.kind) \(.namespace)/\(.name) is outside the Application's namespace";
            "1.12 refuses it";
            "copy \(.kind) \(.name) into namespace \($appns) and drop valuesFrom[].namespace";
            notes) ),
      ( $p.chart.auth.secretRef? // empty | select((.namespace // "") != "" and .namespace != $appns)
        | fx("helm-sources"; "error"; "cross-namespace-credentials"; "Application \($a | nn)";
            "component \($c.name): chart.auth.secretRef \(.namespace)/\(.name) is outside the Application's namespace";
            "1.12 refuses it";
            "copy Secret \(.name) into namespace \($appns) and drop chart.auth.secretRef.namespace";
            notes) ),
      ( ($p.release.namespace // "") as $relns
        | ([($p.valuesFrom // [])[] | select((.namespace // "") == "") | "\(.kind) \(.name)"]
           + [($p.chart.auth.secretRef? // empty) | select((.namespace // "") == "") | "Secret \(.name)"]) as $unset
        | select(($unset | length) > 0)
        | ( select($relns != "" and $relns != $appns and ("helm-sources" | since(.) | not))
            | f("warn"; "cross-namespace-credentials"; "Application \($a | nn)";
                "component \($c.name): sources without a namespace (\($unset | join(", "))); \(vlabel) reads them from the release namespace \($relns), 1.12 only from \($appns)";
                "create \($unset | join(", ")) in namespace \($appns) before upgrading";
                notes) ),
          ( select($topons != null)
            | f("warn"; "cross-namespace-credentials"; "Application \($a | nn)";
                "component \($c.name): sources without a namespace (\($unset | join(", "))); under the topology namespace \($topons) 1.11 reads them from \($topons), 1.12 only from \($appns)";
                "create \($unset | join(", ")) in namespace \($appns) before upgrading";
                notes) ) ) ),
  ( items($apps)[] as $a | $a.metadata.namespace as $appns
    | $a | steps | select(.type | IN("read-config", "list-config", "create-config", "delete-config")) as $s
    | ($s.properties // {}) as $p
    | ( select(($p.namespace // "") != "" and $p.namespace != $appns)
        | ({"read-config": "get", "list-config": "list", "create-config": "create", "delete-config": "delete"}[$s.type]) as $verb
        | select(($s.type == "read-config" and $p.namespace == "vela-system" and shared_config($p.name // "")) or
                 ($s.type == "list-config" and $p.namespace == "vela-system" and shared_template($p.template // "")) | not)
        | if $authn then
            f("warn"; "cross-namespace-credentials"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) reaches config \($p.namespace)/\($p.name // "*"); 1.12 allows it only if the Application's user may \($verb) configs there";
              "grant the Application's user \($verb) on configs.config.oam.dev in namespace \($p.namespace), or move the config to \($appns)";
              notes)
          else
            fx("config-steps"; "error"; "cross-namespace-credentials"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) reaches config \($p.namespace)/\($p.name // "*") outside the Application's namespace";
              "1.12 refuses it";
              "move the config into namespace \($appns) and drop the step's namespace property (a shared config in vela-system made from a vela-system template stays readable)";
              notes)
          end ),
      ( select($s.type == "create-config" and (($p.template // "") | contains("/")))
        | ($p.template | split("/")[0]) as $tns
        | select($tns != "vela-system" and $tns != ($p.namespace // $appns))
        | f("error"; "cross-namespace-credentials"; "Application \($a | nn)";
            "step \($s.name) (create-config) uses template \($p.template) from another namespace; 1.12 refuses a namespace-scoped template from elsewhere";
            "move template \($p.template | split("/")[1]) into vela-system or namespace \($appns), and name it there";
            guide("config-templates-and-configs")) ) ),
  ( items($sdefs)[] | select((.spec.schematic.cue.template // "") | test("tls_config"))
    | f("info"; "cross-namespace-credentials"; "WorkflowStepDefinition \(nn)";
        "its template sets tls_config: 1.12 reads that TLS Secret only from the namespace of the Application running the step";
        "check that each Application using step type \(.metadata.name) has the Secret in its own namespace";
        notes) ),
  ( items($cdefs)[] | . as $d | (.spec.schematic.terraform // {}) as $tf
    | select($tf.type == "remote")
    | $tf.gitCredentialsSecretReference? // empty
    | select((.namespace // "") != "" and .namespace != $d.metadata.namespace)
    | fx("terraform-ssh"; "error"; "cross-namespace-credentials"; "ComponentDefinition \($d | nn)";
        "gitCredentialsSecretReference \(.namespace)/\(.name) is outside the definition's namespace, so the module's variables cannot be read";
        "1.12 refuses it";
        "copy Secret \(.name) into namespace \($d.metadata.namespace) and set gitCredentialsSecretReference.namespace to \($d.metadata.namespace)";
        notes) ),

# 5. Workflow HTTP guard and limits.
  ( items($apps)[] as $a
    | $a | steps as $s
    | ( select($after.DisableWorkflowHTTP) | $s | stepurls | select(length > 0)
        | if $now.DisableWorkflowHTTP // false then
            f("warn"; "http-guard"; "Application \($a | nn)";
              "already refused: step \($s.name) (\($s.type)) makes a workflow HTTP request, and disableWorkflowHTTP is on";
              "turn featureGates.disableWorkflowHTTP off, or remove the step"; notes)
          else
            f("error"; "http-guard"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) makes a workflow HTTP request; with featureGates.disableWorkflowHTTP on in the values 1.12 refuses every one";
              "turn featureGates.disableWorkflowHTTP off, or remove the step"; notes)
          end ),
      ( $s | stepurls[] as $u
        | (if $u.url != null then {host: ($u.url | urlhost), from: ""}
           else ($hostOf["\($a.metadata.namespace)/\($u.secret.name)/\($u.secret.key // "")"] // {skipped: "not collected"})
             | if .host != null then {host: (.host | urlhost), from: " (from Secret \(.namespace)/\(.name))"}
               else {skipped: .skipped, secret: "\($a.metadata.namespace)/\($u.secret.name)"} end end) as $t
        | if $t.skipped != null then
            f("info"; "http-guard"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) takes its \($u.at) from Secret \($t.secret), which could not be read (\($t.skipped)), so its host was not checked";
              "check that the URL in Secret \($t.secret) is not a metadata, link-local, loopback or denied address";
              notes)
          else
            hostdenied($t.host) as $hits | select(($hits | length) > 0)
            | ($hits | map(select(.after))) as $later | select(($later | length) > 0)
            | if any($hits[]; .now) then
                f("warn"; "http-guard"; "Application \($a | nn)";
                  "already refused on \(vlabel): step \($s.name) (\($s.type)) calls \($t.host) at \($u.at)\($t.from), \(([$hits[] | select(.now)] | first).why)";
                  "point \($u.at) at an address the controller may call, such as the Service's cluster DNS name";
                  notes)
              else
                f("error"; "http-guard"; "Application \($a | nn)";
                  "step \($s.name) (\($s.type)) calls \($t.host) at \($u.at)\($t.from), \($later[0].why): the 1.12 HTTP guard refuses it";
                  "point \($u.at) at an address the controller may call, such as the Service's cluster DNS name";
                  notes)
              end
          end ),
      ( $s | select(.type | IN("request", "webhook", "notification")) | .properties.timeout? // empty
        | select(type == "string") as $t | ($t | duration) as $d
        | (if $d == null then "has timeout \"\($t)\", which is not a duration with a unit"
           elif $d <= 0 then "has timeout \"\($t)\", and a timeout must be greater than zero"
           elif $d > $maxAfter then "has timeout \"\($t)\", over the controller's maximum \($maxAfterStr)"
           else null end) as $why
        | select($why != null)
        | ($d == null or $d <= 0 or $d > $maxNow) as $failsNow
        | (if $d == null then "set timeout to a Go duration with a unit, such as \"\($t | if test("^[0-9]+$") then . + "s" else "30s" end)\""
           elif $d <= 0 then "set timeout to a positive duration up to \($maxAfterStr), or leave it out for 3s"
           else "lower timeout to \($maxAfterStr) or less, or raise workflow.http.maxTimeout in the chart values" end) as $fix
        | if $failsNow and ("http-timeout" | since(.)) then
            f("warn"; "http-guard"; "Application \($a | nn)";
              "already refused on \(vlabel) (since \(first_version["http-timeout"])): step \($s.name) (\($s.type)) \($why)"; $fix; notes)
          else
            f("error"; "http-guard"; "Application \($a | nn)"; "step \($s.name) (\($s.type)) \($why); 1.12 refuses it"; $fix; notes)
          end ) ),

# 6. vela-cli outside the namespaces it is allowed in.
  ( select($after.RestrictDefinitionNamespaces)
    | items($apps)[] as $a | $a.metadata.namespace as $appns
    | $a | steps | select(.type | split("@")[0] == "vela-cli") as $s
    | resolve("s"; $s.type; $appns) // {metadata: {namespace: $sysns, name: "vela-cli"}}
    | restrictionsOf("WorkflowStepDefinition"; .) as $r
    | select(allows($r; $appns) | not)
    | f("error"; "vela-cli-namespace"; "Application \($a | nn)";
        "step \($s.name) uses vela-cli in namespace \($appns); 1.12 restricts vela-cli to \(describe($r)) and refuses the next spec change";
        "add \($appns) to definitionRestrictions.overrides.vela-cli.namespaces in your values file, and set the step's serviceAccountName to an account in \($appns) that may do what it runs";
        notes) ),
  # Before 1.11.2 the step's Job ran in vela-system as vela-core's own service account by default.
  ( select("vela-cli-account" | since(.) | not)
    | items($apps)[] as $a
    | $a | steps | select((.type | split("@")[0] == "vela-cli") and (.properties.serviceAccountName // "") == "") as $s
    | f("warn"; "vela-cli-namespace"; "Application \($a | nn)";
        "step \($s.name) names no serviceAccountName: on \(vlabel) its Job runs as vela-core's service account, from 1.11.2 as default in \($a.metadata.namespace)";
        "set serviceAccountName to an account in \($a.metadata.namespace) with the permissions the command needs (kubevela-vela-core, in vela-system only)";
        notes) ),

# 7. Global policy defaults. Global policies run only with both gates on.
  ( select($after.EnableGlobalPolicies and $after.EnableApplicationScopedPolicies)
    | ( select($after.allowNamespaceGlobalPolicies | not)
        | items($pdefs)[] | select(.spec.global == true and .metadata.namespace != $sysns)
        | f("error"; "global-policies"; "PolicyDefinition \(nn)";
            "a global policy outside \($sysns): with controllerArgs.allowNamespaceGlobalPolicies defaulting to false in 1.12 it stops applying";
            "set controllerArgs.allowNamespaceGlobalPolicies: true in your values file, or move the policy to \($sysns)";
            notes) ),
      ( select($after.allowSkipGlobalPolicies | not)
        | items($apps)[] | select(.metadata.annotations["policy.oam.dev/skip-global"] == "true")
        | f("error"; "global-policies"; "Application \(nn)";
            "has policy.oam.dev/skip-global: with controllerArgs.allowSkipGlobalPolicies defaulting to false in 1.12 the annotation is ignored and global policies apply";
            "set controllerArgs.allowSkipGlobalPolicies: true in your values file, or remove the annotation";
            notes) ) ),

# 8. The resource trait sets only what it is given.
  ( items($apps)[] as $a
    | ($a.spec.components // [])[] as $c
    | (($c.traits // [])[] | select(.type | split("@")[0] == "resource")) as $t
    | ($t.properties // {}) as $p
    | select((($p.cpu != null) != ($p.memory != null)) and $p.requests == null and $p.limits == null)
    | (if $p.cpu != null then ["cpu", "memory"] else ["memory", "cpu"] end) as [$set, $other]
    | f("warn"; "resource-trait"; "Application \($a | nn)";
        "component \($c.name): the resource trait sets only \($set); 1.11 ignored a lone \($set) and set no requests or limits, 1.12 sets \($set) requests and limits, so the Pods restart with them";
        "check that \($set) \($p[$set]) is what the workload should get, and add \($other) if it needs one; remove the trait to keep the 1.11 result";
        notes) ),

# 9. Configs as resources.
  ( [items($secrets)[] | select(.metadata.labels["config.oam.dev/type"] != null)] as $configs
    | select(($configs | length) > 0)
    | ([$configs[].metadata.namespace] | unique) as $nss
    | f("warn"; "configs"; "Configs (\($configs | length) in \($nss | length) namespace\(if ($nss | length) == 1 then "" else "s" end))";
        "configs become Config resources; users who manage them need RBAC on configs and configtemplates, which edit and admin do not grant";
        "grant get, list, watch, create, update, patch, delete on configs.config.oam.dev and configtemplates.config.oam.dev to the users and service accounts that manage configs in \($nss | join(", "))";
        guide("config-templates-and-configs")) ),
  ( $tmpls[] | select(.metadata.namespace != "vela-system" and .metadata.labels["config.oam.dev/scope"] == "system")
    | f("warn"; "configs"; "ConfigMap \(nn)";
        "template \(.metadata.name | ltrimstr("config-template-")) is labelled scope system outside vela-system; 1.12 treats it as namespace-scoped, so new configs elsewhere cannot use it";
        "apply the template in vela-system if other namespaces use it, or label it config.oam.dev/scope=namespace";
        guide("config-templates-and-configs")) ),
  ( items($secrets)[] as $s
    | ($s.data["objects-reference"] // "" | if . == "" then [] else (@base64d | try fromjson catch []) end) as $refs
    | [$refs[]? | select((.namespace // "") != $s.metadata.namespace) | "\(.kind) \(.namespace // "")/\(.name)"] as $out
    | select(($out | length) > 0)
    | f("warn"; "configs"; "Secret \($s | nn)";
        "config outputs \($out | join(", ")) are outside its namespace; 1.12 writes outputs into the config's namespace and migration reports it as conflict";
        "keep it as a legacy config, or recreate it in the namespace its outputs belong in";
        guide("config-templates-and-configs")) ),
  ( $tmpls[] as $t | ($t.data.template // "") as $cue
    | select($cue | test("\\bnacos\\s*:"))
    | (($cue | capture("endpoint\\s*:\\s*\\{[^}]*\\bname\\s*:\\s*\"(?<n>[^\"]+)\"") // ($cue | capture("endpoint\\s*:\\s*name\\s*:\\s*\"(?<n>[^\"]+)\"")) // {n: null}).n) as $ep
    | (($cue | capture("endpoint\\s*:\\s*\\{[^}]*\\bnamespace\\s*:\\s*\"(?<n>[^\"]+)\"") // {n: "vela-system"}).n) as $epns
    | ( f("warn"; "configs"; "ConfigMap \($t | nn)";
          "template \($t.metadata.name | ltrimstr("config-template-")) publishes to Nacos; in 1.12 the controller publishes, not the CLI";
          "make sure vela-core's pods can reach the Nacos server named in endpoint config \($epns)/\($ep // "?")";
          guide("config-templates-and-configs")),
        ( select($ep != null) | $sidxs[$epns + "/" + $ep] // empty
          | select(.metadata.annotations["config.oam.dev/sensitive"] == "true" and .metadata.annotations["config.oam.dev/nacos-endpoint"] != "true")
          | f("warn"; "configs"; "Secret \(nn)";
              "a sensitive Nacos endpoint config without config.oam.dev/nacos-endpoint; 1.12 will not publish through it";
              "kubectl annotate secret -n \(.metadata.namespace) \(.metadata.name) config.oam.dev/nacos-endpoint=true";
              guide("config-templates-and-configs")) ),
        ( select($epns != "vela-system" and $epns != $t.metadata.namespace)
          | f("warn"; "configs"; "ConfigMap \($t | nn)";
              "the Nacos endpoint \($epns)/\($ep // "?") is outside vela-system and the template's namespace; 1.12 refuses it for a legacy template";
              "move the endpoint config to vela-system or \($t.metadata.namespace), or re-apply the template as a ConfigTemplate";
              guide("config-templates-and-configs")) ) ) ),
  ( items($secrets)[] as $s | ($s.metadata.labels["config.oam.dev/type"] // "") as $type | select($type != "")
    | ($s.metadata.annotations["config.oam.dev/template-namespace"] // "vela-system") as $tns
    | $tmpls[$tns + "/config-template-" + $type] // empty
    | select(.metadata.annotations["config.oam.dev/sensitive"] == "true" and $s.metadata.annotations["config.oam.dev/sensitive"] != "true")
    | f("warn"; "configs"; "Secret \($s | nn)";
        "made before template \($type) became sensitive; migrated, it is sensitive, and read-config and list-config can no longer read it";
        "read it some other way (mount its Secret) before migrating, or keep it as a legacy config";
        guide("config-templates-and-configs")) ),

# 10. Definition restrictions, from the definition's annotation and what the chart stamps on its
# builtins; one finding per Application.
  ( select($after.RestrictDefinitionNamespaces)
    | items($apps)[] as $a | $a.metadata.namespace as $appns
    | [ ( ($a.spec.components // [])[] | {kind: "ComponentDefinition", k: "c", type: .type},
          ((.traits // [])[] | {kind: "TraitDefinition", k: "t", type: .type}) ),
        ( ($a.spec.policies // [])[] | {kind: "PolicyDefinition", k: "p", type: .type},
          (select(.type == "override") | (.properties.components // [])[]
            | (select(.type != null) | {kind: "ComponentDefinition", k: "c", type: .type}),
              ((.traits // [])[] | {kind: "TraitDefinition", k: "t", type: .type})) ),
        ( $a | steps | select(.type | split("@")[0] != "vela-cli") | {kind: "WorkflowStepDefinition", k: "s", type: .type} )
      | select(.type != null) | . as $u
      | resolve($u.k; $u.type; $appns) // empty
      | . as $d | restrictionsOf($u.kind; $d) as $r
      | select(allows($r; $appns) | not)
      | "\($u.kind) \($d.metadata.name) (\(describe($r)))" ] | unique as $uses
    | select(($uses | length) > 0)
    | f("warn"; "definition-restrictions"; "Application \($a | nn)";
        "uses \($uses | join(", ")), none allowing namespace \($appns); with RestrictDefinitionNamespaces on 1.12 refuses its next spec change";
        "allow \($appns) in each definition's definition.oam.dev/restrict-namespaces annotation or the chart's definitionRestrictions, or move the Application";
        notes) ),

# 11. Webhooks with cert-manager or a supplied Secret.
  ( select(($vals.admissionWebhooks.certManager.enabled // false) == true or ($vals.admissionWebhooks.patch.enabled // true) == false)
    | f("info"; "cert-manager"; "Release \($ns)/\($rel)";
        "the webhooks render with failurePolicy Fail from the first install, so writes are refused until the caBundle is in place";
        "make sure cert-manager's CA injector (or your own caBundle) is ready; raise builtinDefinitions.job.rolloutTimeoutSeconds (300) if injection takes longer";
        guide("runtime-certificates")) ),

# 12. GitOps rendering.
  ( ( [items($argo)[] | select(
        ([.spec.source, (.spec.sources // [])[]] | map(select(. != null)) | any(.chart == "vela-core" or ((.path // "") | test("vela-core")))))] )
      + [items($deploy)[] | select(.metadata.labels["argocd.argoproj.io/instance"] != null or .metadata.annotations["argocd.argoproj.io/tracking-id"] != null)]
    | select(length > 0)
    | f("info"; "gitops"; "\(.[0].kind) \(.[0] | nn)";
        "Argo CD manages vela-core; rendered without cluster access, the webhook failurePolicy renders as Ignore and vela-core sets it at runtime, so Argo CD reports drift";
        "add ignoreDifferences on MutatingWebhookConfiguration and ValidatingWebhookConfiguration for jqPathExpressions .webhooks[].failurePolicy";
        guide("runtime-certificates")) ),

# 13. Images to mirror.
  ( ($vals.image.repository // "oamdev/vela-core") as $repo
    | f("info"; "images"; "Images";
        "1.12 pulls alpine/k8s:1.34.13 (definitions Job), oamdev/cluster-gateway:v1.10.0-alpha.1 and ghcr.io/osscontainertools/kaniko:v1.29.1 (build-push-image), beside \($repo) and oamdev/vela-cli at the 1.12 tag; oamdev/kube-webhook-certgen is no longer used";
        "on an air-gapped cluster, mirror these before upgrading and point builtinDefinitions.job.image.* and multicluster.clusterGateway.image.* at the mirror";
        guide("built-in-definitions")) )
]
| map(select(. != null))
| unique_by([.level, .check, .object, .problem])
| sort_by([(.level as $l | ["error", "warn", "info"] | index($l)), .check, .object])
JQ

findings="$(jq -n \
	--arg ns "$namespace" --arg rel "$release" --argjson helmRead "$helm_read" \
	--slurpfile apps "$tmp/applications.json" --slurpfile wfs "$tmp/workflows.json" \
	--slurpfile cdefs "$tmp/componentdefinitions.json" --slurpfile tdefs "$tmp/traitdefinitions.json" \
	--slurpfile pdefs "$tmp/policydefinitions.json" --slurpfile sdefs "$tmp/workflowstepdefinitions.json" \
	--slurpfile cms "$tmp/configmaps.json" --slurpfile secrets "$tmp/secrets.json" \
	--slurpfile crd "$tmp/crd-applications.json" --slurpfile deploy "$tmp/deployment.json" \
	--slurpfile argo "$tmp/argocd-applications.json" --slurpfile revs "$tmp/definitionrevisions.json" \
	--slurpfile nss "$tmp/namespaces.json" --slurpfile deny "$tmp/deny-configmaps.json" \
	--slurpfile shosts "$tmp/secret-hosts.json" --slurpfile resolved "$tmp/resolved.json" \
	--slurpfile values "$tmp/helm-values.json" --slurpfile release "$tmp/helm-release.json" \
	"$lib$analysis")" || { echo "error: the analysis failed (see jq's message above)" >&2; exit 2; }

errors="$(echo "$findings" | jq '[.[] | select(.level == "error")] | length')"
warnings="$(echo "$findings" | jq '[.[] | select(.level == "warn")] | length')"
infos="$(echo "$findings" | jq '[.[] | select(.level == "info")] | length')"
summary="$errors errors, $warnings warnings, $infos info"

if [ "$format" = json ]; then
	echo "$findings" | jq --argjson e "$errors" --argjson w "$warnings" --argjson i "$infos" \
		'{findings: ., summary: {errors: $e, warnings: $w, info: $i}}'
else
	if [ "$verbosity" = verbose ]; then
		for check in trait-conflicts helm-values crds cross-namespace-credentials http-guard vela-cli-namespace \
			global-policies resource-trait configs definition-restrictions cert-manager gitops images; do
			echo "check $check: $(echo "$findings" | jq --arg c "$check" '[.[] | select(.check == $c)] | length') findings"
		done
	fi
	if [ "$verbosity" != quiet ]; then
		echo "$findings" | jq -r '.[] | "\(.level | ascii_upcase) [\(.check)] \(.object)\n  problem: \(.problem)\n  fix: \(.fix)\n  see: \(.link)"'
	fi
	echo "$summary"
fi

[ "$errors" -eq 0 ] || exit 1
