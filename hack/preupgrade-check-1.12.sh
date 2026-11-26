#!/usr/bin/env bash
# Checks a running KubeVela 1.11 cluster for what the 1.12 upgrade refuses or changes, and prints a
# fix for each finding. It only reads: kubectl get and list, helm get values and helm list.
#
# Usage:
#   hack/preupgrade-check-1.12.sh [--release NAME] [--namespace NS] [--json] [-v | -q] [--from-dir DIR]
#     --release     Helm release name (default kubevela)
#     --namespace   release namespace (default vela-system)
#     --json        print the findings as JSON
#     -v            also list each check with its finding count
#     -q            print only the summary line
#     --from-dir    read captured files from DIR instead of the cluster (see collect below for names)
#
# Exit codes: 0 no errors, 1 errors found, 2 the check could not run (a missing tool, no cluster,
# KubeVela not installed).
#
# Needs kubectl and jq. helm is optional: without it the release values are not read, and the
# checks that depend on them assume the chart defaults.
set -euo pipefail

release=kubevela
namespace=vela-system
format=text
verbosity=normal
from_dir=""

while [ $# -gt 0 ]; do
	case "$1" in
	--release) release="$2"; shift ;;
	--namespace) namespace="$2"; shift ;;
	--json) format=json ;;
	-v | --verbose) verbosity=verbose ;;
	-q | --quiet) verbosity=quiet ;;
	--from-dir) from_dir="$2"; shift ;;
	-h | --help) sed -n '2,19p' "$0"; exit 0 ;;
	*) echo "error: unknown argument $1" >&2; exit 2 ;;
	esac
	shift
done

command -v jq >/dev/null || { echo "error: jq is not installed" >&2; exit 2; }

tmp="$(mktemp -d)"
chmod 700 "$tmp"
trap 'rm -rf "$tmp"' EXIT

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
	local kind file
	for kind in applications.core.oam.dev:applications workflows.core.oam.dev:workflows \
		componentdefinitions.core.oam.dev:componentdefinitions traitdefinitions.core.oam.dev:traitdefinitions \
		policydefinitions.core.oam.dev:policydefinitions workflowstepdefinitions.core.oam.dev:workflowstepdefinitions; do
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
	if command -v helm >/dev/null; then
		if ! helm get values "$release" -n "$namespace" -o json >"$tmp/helm-values.json" 2>"$tmp/err"; then
			echo "error: cannot read the values of release $release in $namespace: $(cat "$tmp/err")" >&2
			exit 2
		fi
		helm list -n "$namespace" --filter "^${release}\$" -o json >"$tmp/helm-release.json"
	fi
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
	configmaps secrets deployment argocd-applications; do
	[ -s "$tmp/$f.json" ] || echo '{"items":[]}' >"$tmp/$f.json"
done
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
def items($f): ($f[0] // {}) | (.items // []);
def nn: "\(.metadata.namespace)/\(.metadata.name)";
def f($level; $check; $object; $problem; $fix; $link):
  {level: $level, check: $check, object: $object, problem: $problem, fix: $fix, link: $link};
def trim: gsub("^\\s+|\\s+$"; "");

($values[0] // {}) as $vals
| ($vals.systemDefinitionNamespace // $ns | if . == "" or . == null then $ns else . end) as $sysns
| (items($deploy)[0].spec.template.spec.containers[0].args // []) as $args
| ([$args[] | select(startswith("--feature-gates=")) | ltrimstr("--feature-gates=") | split(",")[]
    | select(contains("=")) | split("=") | {(.[0]): (.[1] == "true")}] | add // {}) as $gates
| ([$args[] | select(startswith("--max-workflow-http-timeout=")) | ltrimstr("--max-workflow-http-timeout=")] | last
    // $vals.workflow.http.maxTimeout // "60s") as $maxTimeoutStr

# Definitions resolve as GetDefinition does: the Application's namespace first, then the system one.
| (reduce items($cdefs)[] as $d ({}; .[$d | nn] = $d)) as $cidx
| (reduce items($tdefs)[] as $d ({}; .[$d | nn] = $d)) as $tidx
| (reduce items($pdefs)[] as $d ({}; .[$d | nn] = $d)) as $pidx
| (reduce items($sdefs)[] as $d ({}; .[$d | nn] = $d)) as $sidx
| (reduce items($wfs)[] as $d ({}; .[$d | nn] = $d)) as $wfidx
| def resolve($idx; $type; $appns): ($type | sub("@.*$"; "")) as $n | ($idx[$appns + "/" + $n] // $idx[$sysns + "/" + $n]);

# Go's time.ParseDuration, to seconds; null when it does not parse.
def duration:
  if . == "0" or . == "+0" or . == "-0" then 0
  elif test("^[+-]?(([0-9]+(\\.[0-9]*)?|\\.[0-9]+)(ns|us|µs|μs|ms|s|m|h))+$") then
    (if startswith("-") then -1 else 1 end) as $sign
    | ([scan("([0-9]*\\.?[0-9]*)(ns|us|µs|μs|ms|s|m|h)") | (.[0] | tonumber) *
        ({"ns": 1e-9, "us": 1e-6, "µs": 1e-6, "μs": 1e-6, "ms": 1e-3, "s": 1, "m": 60, "h": 3600}[.[1]])] | add) * $sign
  else null end;
($maxTimeoutStr | duration // 60) as $maxTimeout

# The steps an Application runs: its own, or those of the Workflow it references, with sub-steps.
| def appsteps:
    .metadata.namespace as $appns
    | (if ((.spec.workflow.steps // []) | length) > 0 then .spec.workflow.steps
       elif (.spec.workflow.ref // "") != "" then
         (($wfidx[$appns + "/" + .spec.workflow.ref] // $wfidx[$sysns + "/" + .spec.workflow.ref]).steps // [])
       else [] end)
    | .[] | ., (.subSteps // [])[];

# A Kubernetes label selector (labels.Parse) against a label map. Set-based terms and equality
# terms; a term this does not understand matches nothing, as an unparseable selector does.
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
  elif test("!=") then (split("!=") | map(trim)) as [$k, $v] | ($l[$k] != $v)
  elif test("=") then (split("=") | map(select(. != "")) | map(trim)) as [$k, $v] | ($l[$k] == $v)
  elif test("^[A-Za-z0-9./_-]+$") then ($l | has(.))
  else false end;
def selmatch($expr; $l):
  ($expr | trim) as $e
  | if $e == "" then true
    else [$e | scan("(?:[^,(]|\\([^)]*\\))+") | trim | select(. != "")] | all(termmatch($l)) end;

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

# The outbound HTTP guard (kubevela/workflow httpguard plus cmd/core/app/http_policy.go): the
# reason a host is refused, or null.
def urlhost:
  sub("^[A-Za-z][A-Za-z0-9+.-]*://"; "") | sub("[/?#].*$"; "") | sub("^.*@"; "")
  | if startswith("[") then (capture("^\\[(?<h>[^\\]]*)\\]").h // "") else sub(":[0-9]*$"; "") end
  | ascii_downcase | rtrimstr(".");
def v4blocked($private):
  (split(".") | map(tonumber)) as $o
  | if $o[0] == 169 and $o[1] == 254 then "a link-local address (169.254.0.0/16, cloud metadata)"
    elif $o == [100, 100, 100, 200] then "a cloud metadata address (100.100.100.200)"
    elif $o[0] == 127 then "a loopback address (127.0.0.0/8)"
    elif $o[0] == 0 then "an unspecified address (0.0.0.0/8)"
    elif $private and ($o[0] == 10 or ($o[0] == 172 and $o[1] >= 16 and $o[1] <= 31) or ($o[0] == 192 and $o[1] == 168))
      then "a private address (blockPrivateHTTPAddresses is on)"
    else null end;
def v6groups:
  (if test("::") then (split("::") as [$a, $b]
      | ($a | if . == "" then [] else split(":") end) as $x
      | ($b | if . == "" then [] else split(":") end) as $y
      | $x + [range(8 - ($x | length) - ($y | length)) | "0"] + $y)
   else split(":") end) | map(("0000" + .)[-4:] | ascii_downcase);
def hostblocked($private):
  . as $h
  | if $h == "metadata.google.internal" then "a cloud metadata host (metadata.google.internal)"
    elif $h == "localhost" or ($h | endswith(".localhost")) then "localhost, which resolves to a loopback address"
    elif $h | test("^[0-9]{1,3}(\\.[0-9]{1,3}){3}$") then $h | v4blocked($private)
    elif $h | test("^::ffff:[0-9]{1,3}(\\.[0-9]{1,3}){3}$") then $h | ltrimstr("::ffff:") | v4blocked($private)
    elif $h | test("^[0-9a-f:]+$") and test(":") then
      ($h | v6groups) as $g
      | if ($g | length) != 8 then null
        elif $g == ["0000","0000","0000","0000","0000","0000","0000","0000"] then "the unspecified address (::)"
        elif $g == ["0000","0000","0000","0000","0000","0000","0000","0001"] then "the loopback address (::1)"
        elif ($g[0] | test("^fe[89ab]")) then "a link-local address (fe80::/10)"
        elif $g == ["fd00","0ec2","0000","0000","0000","0000","0000","0254"] then "a cloud metadata address (fd00:ec2::254)"
        elif $g[0:3] == ["0064","ff9b","0001"] then "a local-use NAT64 address (64:ff9b:1::/48)"
        elif $g[0:6] == ["0064","ff9b","0000","0000","0000","0000"] then
          ([$g[6], $g[7]] | map(explode | map(if . >= 97 then . - 87 else . - 48 end) | reduce .[] as $d (0; . * 16 + $d))
            | "\(.[0] / 256 | floor).\(.[0] % 256).\(.[1] / 256 | floor).\(.[1] % 256)") | v4blocked($private)
        elif $private and ($g[0] | test("^f[cd]")) then "a unique local address (fc00::/7; blockPrivateHTTPAddresses is on)"
        else null end
    else null end;

# The URLs an HTTP step calls, with where each sits in its properties.
def stepurls:
  .properties as $p
  | if .type == "request" then [{at: "url", url: $p.url}]
    elif .type == "webhook" then [{at: "url.value", url: $p.url.value?}]
    elif .type == "notification" then [("lark", "dingding", "slack") as $k | {at: "\($k).url.value", url: $p[$k].url.value?}]
    else [] end
  | map(select(.url | type == "string"));

($gates.BlockPrivateHTTPAddresses // false) as $private
| ($gates.AuthenticateApplication // false) as $authn
| (reduce items($secrets)[] as $s ({}; .[$s | nn] = $s)) as $sidxs
| (reduce (items($cms)[] | select(.metadata.name | startswith("config-template-"))) as $c ({}; .[$c | nn] = $c)) as $tmpls
| def shared_config($name):
    ($sidxs["vela-system/" + $name].metadata.labels["config.oam.dev/type"] // "") as $t
    | $t != "" and ($tmpls["vela-system/config-template-" + $t] != null);
  def shared_template($template): ($template | split("/") | last) as $t | $tmpls["vela-system/config-template-" + $t] != null;

[
# 1. Trait conflicts.
  ( items($apps)[] as $a | $a.metadata.namespace as $appns
    | ($a.spec.components // [])[] as $c
    | [($c.traits // [])[] | resolve($tidx; .type; $appns) | select(. != null)] as $defs
    | range(0; $defs | length) as $i | range($i + 1; $defs | length) as $j
    | select(conflicts($defs[$i]; $defs[$j]) or conflicts($defs[$j]; $defs[$i]))
    | f("error"; "trait-conflicts"; "Application \($a | nn)";
        "component \($c.name): traits \($defs[$i].metadata.name) and \($defs[$j].metadata.name) conflict (conflictsWith); 1.12 refuses the next spec change";
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
        "the release values were not read (no helm), so the checks that use them assume the chart defaults";
        "install helm and run again, or check the values yourself: no image.tag pin, and the global policy and cert-manager settings";
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
        | f("error"; "cross-namespace-credentials"; "Application \($a | nn)";
            "component \($c.name): valuesFrom \(.kind) \(.namespace)/\(.name) is outside the Application's namespace; 1.12 refuses it";
            "copy \(.kind) \(.name) into namespace \($appns) and drop valuesFrom[].namespace";
            notes) ),
      ( $p.chart.auth.secretRef? // empty | select((.namespace // "") != "" and .namespace != $appns)
        | f("error"; "cross-namespace-credentials"; "Application \($a | nn)";
            "component \($c.name): chart.auth.secretRef \(.namespace)/\(.name) is outside the Application's namespace; 1.12 refuses it";
            "copy Secret \(.name) into namespace \($appns) and drop chart.auth.secretRef.namespace";
            notes) ),
      ( ($p.release.namespace // "") as $relns
        | ([($p.valuesFrom // [])[] | select((.namespace // "") == "") | "\(.kind) \(.name)"]
           + [($p.chart.auth.secretRef? // empty) | select((.namespace // "") == "") | "Secret \(.name)"]) as $unset
        | select(($unset | length) > 0)
        | ( select($relns != "" and $relns != $appns)
            | f("warn"; "cross-namespace-credentials"; "Application \($a | nn)";
                "component \($c.name): sources without a namespace (\($unset | join(", "))); 1.11.0 and 1.11.1 read them from the release namespace \($relns), 1.12 only from \($appns)";
                "create \($unset | join(", ")) in namespace \($appns) before upgrading";
                notes) ),
          ( select($topons != null)
            | f("warn"; "cross-namespace-credentials"; "Application \($a | nn)";
                "component \($c.name): sources without a namespace (\($unset | join(", "))); under the topology namespace \($topons) 1.11.2 read them from \($topons), 1.12 only from \($appns)";
                "create \($unset | join(", ")) in namespace \($appns) before upgrading";
                notes) ) ) ),
  ( items($apps)[] as $a | $a.metadata.namespace as $appns
    | $a | appsteps | select(.type | IN("read-config", "list-config", "create-config", "delete-config")) as $s
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
            f("error"; "cross-namespace-credentials"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) reaches config \($p.namespace)/\($p.name // "*"); 1.12 keeps config steps to the Application's namespace";
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
    | f("error"; "cross-namespace-credentials"; "ComponentDefinition \($d | nn)";
        "gitCredentialsSecretReference \(.namespace)/\(.name) is outside the definition's namespace; 1.12 refuses it and cannot read the module's variables";
        "copy Secret \(.name) into namespace \($d.metadata.namespace) and set gitCredentialsSecretReference.namespace to \($d.metadata.namespace)";
        notes) ),

# 5. Workflow HTTP guard and limits.
  ( items($apps)[] as $a
    | $a | appsteps as $s
    | ( $s | stepurls[] as $u | ($u.url | urlhost) as $h | ($h | hostblocked($private)) as $why
        | select($why != null)
        | f("error"; "http-guard"; "Application \($a | nn)";
            "step \($s.name) (\($s.type)) calls \($h) at \($u.at), \($why): the 1.12 HTTP guard refuses it";
            "point \($u.at) at an address the controller may call, such as the Service's cluster DNS name";
            notes) ),
      ( $s | select(.type | IN("request", "webhook", "notification")) | .properties.timeout? // empty
        | select(type == "string") as $t | ($t | duration) as $d
        | if $d == null then
            f("error"; "http-guard"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) has timeout \"\($t)\", which is not a duration with a unit";
              "set timeout to a Go duration with a unit, such as \"\($t | if test("^[0-9]+$") then . + "s" else "30s" end)\"";
              notes)
          elif $d <= 0 then
            f("error"; "http-guard"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) has timeout \"\($t)\"; 1.12 refuses a timeout of zero or less";
              "set timeout to a positive duration up to \($maxTimeoutStr), or leave it out for 3s";
              notes)
          elif $d > $maxTimeout then
            f("error"; "http-guard"; "Application \($a | nn)";
              "step \($s.name) (\($s.type)) has timeout \"\($t)\", over the controller's maximum \($maxTimeoutStr)";
              "lower timeout to \($maxTimeoutStr) or less, or raise workflow.http.maxTimeout in the chart values";
              notes)
          else empty end ) ),

# 6. vela-cli outside the namespaces it is allowed in.
  ( ($vals.definitionRestrictions.overrides["vela-cli"].namespaces // ["vela-system"]) as $allowed
    | items($apps)[] as $a | $a.metadata.namespace as $appns
    | $a | appsteps | select(.type | sub("@.*$"; "") == "vela-cli") as $s
    | select($sidx[$appns + "/vela-cli"] == null or $appns == $sysns)
    | select(any($allowed[]; . as $p | $appns | globmatch($p)) | not)
    | f("error"; "vela-cli-namespace"; "Application \($a | nn)";
        "step \($s.name) uses vela-cli in namespace \($appns); 1.12 restricts vela-cli to \($allowed | join(", "))";
        "add \($appns) to definitionRestrictions.overrides.vela-cli.namespaces in your values file, and set the step's serviceAccountName to an account in \($appns) that may do what it runs";
        notes) ),
  # Before 1.11.2 the step's Job ran in vela-system as vela-core's own service account by default.
  ( ($release[0][0].chart // "" | ltrimstr("vela-core-")) as $chart
    | select($chart | test("^1\\.11\\.[01]$"))
    | items($apps)[] as $a
    | $a | appsteps | select((.type | sub("@.*$"; "") == "vela-cli") and (.properties.serviceAccountName // "") == "") as $s
    | f("warn"; "vela-cli-namespace"; "Application \($a | nn)";
        "step \($s.name) names no serviceAccountName: on chart \($chart) its Job ran as vela-core's service account, from 1.11.2 it runs as default in \($a.metadata.namespace)";
        "set serviceAccountName to an account in \($a.metadata.namespace) with the permissions the command needs (kubevela-vela-core, in vela-system only)";
        notes) ),

# 7. Global policy defaults.
  ( ($gates.EnableGlobalPolicies // false) as $globals
    | select($globals)
    | ( select(($vals.controllerArgs.allowNamespaceGlobalPolicies // false) != true)
        | items($pdefs)[] | select(.spec.global == true and .metadata.namespace != $sysns)
        | f("error"; "global-policies"; "PolicyDefinition \(nn)";
            "a global policy outside \($sysns): with controllerArgs.allowNamespaceGlobalPolicies defaulting to false in 1.12 it stops applying";
            "set controllerArgs.allowNamespaceGlobalPolicies: true in your values file, or move the policy to \($sysns)";
            notes) ),
      ( select(($vals.controllerArgs.allowSkipGlobalPolicies // false) != true)
        | items($apps)[] | select(.metadata.annotations["policy.oam.dev/skip-global"] == "true")
        | f("error"; "global-policies"; "Application \(nn)";
            "has policy.oam.dev/skip-global: with controllerArgs.allowSkipGlobalPolicies defaulting to false in 1.12 the annotation is ignored and global policies apply";
            "set controllerArgs.allowSkipGlobalPolicies: true in your values file, or remove the annotation";
            notes) ) ),

# 8. The resource trait sets only what it is given.
  ( items($apps)[] as $a
    | ($a.spec.components // [])[] as $c
    | (($c.traits // [])[] | select(.type | sub("@.*$"; "") == "resource")) as $t
    | ($t.properties // {}) as $p
    | select((($p.cpu != null) != ($p.memory != null)) and $p.requests == null and $p.limits == null)
    | (if $p.cpu != null then ["cpu", "memory", "2048Mi"] else ["memory", "cpu", "1"] end) as [$set, $other, $default]
    | f("warn"; "resource-trait"; "Application \($a | nn)";
        "component \($c.name): the resource trait sets only \($set); 1.11 also set \($other) to \($default), 1.12 leaves \($other) as the workload has it";
        "add \($other): \($default) to the trait's properties to keep the 1.11 result";
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

# 10. Definition restrictions.
  ( items($apps)[] as $a | $a.metadata.namespace as $appns
    | ( ( ($a.spec.components // [])[] | {kind: "ComponentDefinition", type: .type, idx: "c"},
          ((.traits // [])[] | {kind: "TraitDefinition", type: .type, idx: "t"}) ),
        ( ($a.spec.policies // [])[] | {kind: "PolicyDefinition", type: .type, idx: "p"},
          (select(.type == "override") | (.properties.components // [])[]
            | (select(.type != null) | {kind: "ComponentDefinition", type: .type, idx: "c"}),
              ((.traits // [])[] | {kind: "TraitDefinition", type: .type, idx: "t"})) ),
        ( $a | appsteps | {kind: "WorkflowStepDefinition", type: .type, idx: "s"} ) )
    | . as $u
    | ({"c": $cidx, "t": $tidx, "p": $pidx, "s": $sidx}[$u.idx]) as $idx
    | resolve($idx; $u.type; $appns) // empty
    | . as $d
    | ([(.metadata.annotations["definition.oam.dev/restrict-namespaces"] // "") | split(",")[] | trim | select(. != "")]) as $pats
    | select(($pats | length) > 0)
    | select(any($pats[]; . as $p | $appns | globmatch($p)) | not)
    | f("warn"; "definition-restrictions"; "Application \($a | nn)";
        "uses \($u.kind) \($d.metadata.name), restricted to \($pats | join(", ")); with RestrictDefinitionNamespaces on by default 1.12 refuses its next spec change";
        "add \($appns) to the definition's definition.oam.dev/restrict-namespaces annotation, or move the Application";
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
	--slurpfile argo "$tmp/argocd-applications.json" \
	--slurpfile values "$tmp/helm-values.json" --slurpfile release "$tmp/helm-release.json" \
	"$analysis")" || { echo "error: the analysis failed (see jq's message above)" >&2; exit 2; }

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
