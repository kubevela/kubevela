#!/usr/bin/env bash
# Live regression matrix for affinity scheduling (#5362 + addon/module/helmchart/webservice).
# Requires: k3d cluster addon-sched, local-addons/local-modules registries, bin/vela with scheduling flags.
set -uo pipefail
# Continue across case failures; exit non-zero only at the end if any failed.

ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
REG="$(cd "$(dirname "$0")" && pwd)"
VELA="${ROOT}/bin/vela"
export KUBECONFIG="${KUBECONFIG:-$(k3d kubeconfig write addon-sched)}"
INFRA_NODE="k3d-addon-sched-agent-0"
PASS=0
FAIL=0
RESULTS=()

log() { printf '%s\n' "$*"; }
ok()  { PASS=$((PASS+1)); RESULTS+=("PASS  $1"); log "PASS  $1"; }
bad() { FAIL=$((FAIL+1)); RESULTS+=("FAIL  $1 — $2"); log "FAIL  $1 — $2"; }

wait_app() {
  local ns="$1" name="$2" want="${3:-running}" timeout="${4:-120}"
  local i phase
  for ((i=0; i<timeout; i+=3)); do
    phase=$(kubectl get app -n "$ns" "$name" -o jsonpath='{.status.status}' 2>/dev/null || echo missing)
    if [[ "$phase" == "$want" ]]; then return 0; fi
    sleep 3
  done
  log "  app $ns/$name phase=$phase (want $want)"
  return 1
}

deploy_ns() {
  local name="$1"
  kubectl get deploy -A -o json | python3 -c "
import json,sys
name='$name'
items=json.load(sys.stdin)['items']
for d in items:
  if d['metadata']['name']==name or d['metadata']['name'].startswith(name):
    print(d['metadata']['namespace']+'/'+d['metadata']['name'])
"
}

assert_deploy_on_infra() {
  local case="$1" deploy="$2" ns_hint="${3:-}"
  local found=0
  while read -r loc; do
    [[ -z "$loc" ]] && continue
    local ns="${loc%/*}" name="${loc#*/}"
    [[ -n "$ns_hint" && "$ns" != "$ns_hint" ]] && continue
    found=1
    local check
    check=$(kubectl get deploy -n "$ns" "$name" -o json | python3 -c "
import json,sys
d=json.load(sys.stdin)
spec=d['spec']['template']['spec']
ns=spec.get('nodeSelector') or {}
aff=spec.get('affinity') or {}
print('ns='+json.dumps(ns,separators=(',',':')))
print('has_aff='+str(bool(aff)).lower())
" 2>/dev/null)
    local pod_line
    pod_line=$(kubectl get pods -n "$ns" -o wide --no-headers 2>/dev/null | awk -v n="$name" '$1 ~ n {print $1,$3,$7; exit}')
    local pod_node pod_phase
    pod_node=$(echo "$pod_line" | awk '{print $3}')
    pod_phase=$(echo "$pod_line" | awk '{print $2}')
    local ns_ok=0 aff_ok=0
    echo "$check" | grep -q '"disktype":"ssd"' && echo "$check" | grep -q '"node-type":"infra"' && ns_ok=1
    echo "$check" | grep -q 'has_aff=true' && aff_ok=1
    if [[ "$pod_node" == "$INFRA_NODE" && ( "$ns_ok" -eq 1 || "$aff_ok" -eq 1 ) ]]; then
      ok "$case ($ns/$name → $INFRA_NODE, phase=$pod_phase)"
      return 0
    fi
    if [[ "$ns_ok" -eq 1 || "$aff_ok" -eq 1 ]]; then
      # scheduling fields present; wait briefly for bind
      sleep 5
      pod_line=$(kubectl get pods -n "$ns" -o wide --no-headers 2>/dev/null | awk -v n="$name" '$1 ~ n {print $1,$3,$7; exit}')
      pod_node=$(echo "$pod_line" | awk '{print $3}')
      if [[ "$pod_node" == "$INFRA_NODE" ]]; then
        ok "$case ($ns/$name → $INFRA_NODE)"
        return 0
      fi
    fi
    bad "$case" "deploy $ns/$name check=$check pod='$pod_line'"
    return 1
  done < <(deploy_ns "$deploy")
  if [[ "$found" -eq 0 ]]; then
    bad "$case" "deploy $deploy not found"
    return 1
  fi
}

assert_pending() {
  local case="$1" deploy="$2"
  local loc ns name phase reason
  loc=$(deploy_ns "$deploy" | head -1)
  if [[ -z "$loc" ]]; then bad "$case" "deploy missing"; return 1; fi
  ns="${loc%/*}"; name="${loc#*/}"
  for ((i=0; i<60; i+=3)); do
    phase=$(kubectl get pods -n "$ns" -o json | python3 -c "
import json,sys
name='$name'
for p in json.load(sys.stdin)['items']:
  if name in p['metadata']['name']:
    print(p['status'].get('phase',''))
    for c in p['status'].get('conditions',[]) or []:
      if c.get('type')=='PodScheduled' and c.get('status')=='False':
        print(c.get('reason',''))
    break
")
    # PodScheduled=False reason is usually Unschedulable (message may mention
    # FailedScheduling); either means the bad selector worked.
    if echo "$phase" | grep -Eq 'FailedScheduling|Unschedulable'; then
      ok "$case (Pending/unschedulable as expected)"
      return 0
    fi
    sleep 3
  done
  bad "$case" "expected Unschedulable/FailedScheduling, got: $phase"
}

assert_scheduled() {
  # Pass when the Deployment exists and a pod was bound to a node (image pull
  # failures from TLS interceptors are orthogonal to scheduling).
  local case="$1" deploy="$2" want_toleration="${3:-}"
  local loc ns name
  loc=$(deploy_ns "$deploy" | head -1)
  if [[ -z "$loc" ]]; then bad "$case" "deploy missing"; return 1; fi
  ns="${loc%/*}"; name="${loc#*/}"
  local info
  info=$(kubectl get pods -n "$ns" -o json | python3 -c "
import json,sys
name='$name'
want_tol='$want_toleration'
for p in json.load(sys.stdin)['items']:
  if name not in p['metadata']['name']:
    continue
  node=p.get('spec',{}).get('nodeName','')
  tols=p.get('spec',{}).get('tolerations') or []
  scheduled=any(c.get('type')=='PodScheduled' and c.get('status')=='True' for c in (p.get('status',{}).get('conditions') or []))
  has_tol=True
  if want_tol:
    has_tol=any(t.get('key')==want_tol for t in tols)
  print(f'node={node} scheduled={scheduled} tol={has_tol}')
  break
else:
  print('missing')
")
  if echo "$info" | grep -q 'scheduled=True' && echo "$info" | grep -q 'tol=True'; then
    ok "$case ($info)"
    return 0
  fi
  bad "$case" "$info"
}

assert_running_any() {
  local case="$1" appns="$2" app="$3" deploy="$4"
  # Prefer healthy running; fall back to scheduled (handles ImagePullBackOff under TLS intercept).
  if wait_app "$appns" "$app" running 45; then
    ok "$case (running)"
    return 0
  fi
  assert_scheduled "$case" "$deploy"
}

assert_no_trait_on_ns_helper() {
  local case="$1" child_app="$2"
  local comps
  comps=$(kubectl get app -n vela-system "$child_app" -o json 2>/dev/null || echo '{}')
  echo "$comps" | python3 -c "
import json,sys
app=json.load(sys.stdin)
comps=app.get('spec',{}).get('components',[])
fail=[]
for c in comps:
  name=c.get('name','')
  traits=c.get('traits') or []
  if name.endswith('-ns') or name.endswith('-namespace'):
    if any(t.get('type')=='affinity' for t in traits):
      fail.append(name)
print('FAIL:'+','.join(fail) if fail else 'OK')
" | grep -q '^OK$' && ok "$case" || bad "$case" "ns helper has affinity trait"
}

cleanup_reg() {
  log "== cleanup prior regression apps =="
  kubectl delete app -n default -l regression.kubevela.io/suite=addon-scheduling --wait=false 2>/dev/null || true
  kubectl delete app -n vela-system -l regression.kubevela.io/suite=addon-scheduling --wait=false 2>/dev/null || true
  # also clear leftover demo apps that collide with addon/module names
  # Keep live-helmchart / cert-manager release for H-* mutations (avoids re-pull).
  for a in live-addon live-module live-embed-2 live-webservice \
           addon-sched-demo module-ctrl-kit module-gadget-kit-inline module-widget-kit-inline \
           addon-kit-suite-inline reg-a-ns reg-a-na reg-a-combo reg-m-ns reg-m-na \
           reg-h-ns reg-h-na reg-h-over reg-embed reg-cli-ns reg-cli-file; do
    kubectl delete app -n vela-system "$a" --wait=false 2>/dev/null || true
    kubectl delete app -n default "$a" --wait=false 2>/dev/null || true
  done
  sleep 5
  if ! kubectl get app -n vela-system live-helmchart >/dev/null 2>&1; then
    kubectl apply -f "$REG/../helmchart.yaml"
    # rename to live-helmchart for H-* helpers
    kubectl get app -n vela-system helmchart-scheduling-example -o json 2>/dev/null | \
      python3 -c 'import json,sys; a=json.load(sys.stdin); a["metadata"]["name"]="live-helmchart";
a.pop("status",None); a["metadata"].pop("resourceVersion",None); a["metadata"].pop("uid",None);
a["metadata"].pop("creationTimestamp",None); a["metadata"].pop("generation",None);
json.dump(a,sys.stdout)' 2>/dev/null | kubectl apply -f - || \
      kubectl apply -f "$REG/30-helmchart-nodeSelector.yaml"
  fi
}

run_webservice() {
  log "== webservice permutations =="
  kubectl apply -f "$REG/01-webservice-nodeSelector.yaml"
  kubectl apply -f "$REG/02-webservice-nodeAffinity.yaml"
  kubectl apply -f "$REG/03-webservice-tolerations.yaml"
  kubectl apply -f "$REG/04-webservice-combo.yaml"
  kubectl apply -f "$REG/05-webservice-bad-selector.yaml"
  kubectl apply -f "$REG/06-webservice-none.yaml"

  wait_app default reg-w-ns running 90 || true
  wait_app default reg-w-na running 90 || true
  wait_app default reg-w-tol running 90 || true
  wait_app default reg-w-combo running 90 || true
  wait_app default reg-w-none running 90 || true
  # bad may stay rendering/running unhealthy
  sleep 5

  assert_deploy_on_infra "W-NS" "reg-w-ns" default
  assert_deploy_on_infra "W-NA" "reg-w-na" default
  assert_scheduled "W-TOL" "reg-w-tol" "dedicated"
  assert_deploy_on_infra "W-COMBO" "reg-w-combo" default
  assert_pending "W-BAD" "reg-w-bad"
  assert_scheduled "W-NONE" "reg-w-none"
}

run_one_addon() {
  local label="$1" appname="$2" file="$3"
  kubectl delete app -n vela-system addon-sched-demo --wait=true 2>/dev/null || true
  kubectl delete app -n vela-system "$appname" --wait=true 2>/dev/null || true
  sleep 2
  kubectl apply -f "$file"
  if ! wait_app vela-system "$appname" running 150; then
    bad "$label" "parent app not running"
    kubectl get app -n vela-system "$appname" -o yaml | tail -40 || true
    return
  fi
  sleep 5
  assert_deploy_on_infra "$label" "sched-demo-ctrl" vela-system
  assert_no_trait_on_ns_helper "${label}-skip-ns" "addon-sched-demo"
}

run_addons() {
  log "== addon permutations (sequential; shared sched-demo) =="
  run_one_addon "A-NS" "reg-a-ns" "$REG/10-addon-nodeSelector.yaml"
  kubectl delete app -n vela-system reg-a-ns --wait=false 2>/dev/null || true
  run_one_addon "A-NA" "reg-a-na" "$REG/11-addon-nodeAffinity.yaml"
  kubectl delete app -n vela-system reg-a-na --wait=false 2>/dev/null || true
  run_one_addon "A-COMBO" "reg-a-combo" "$REG/12-addon-combo.yaml"
}

run_modules() {
  log "== module permutations (sequential; shared ctrl-kit) =="
  kubectl delete app -n vela-system module-ctrl-kit reg-m-ns reg-m-na live-module --wait=false 2>/dev/null || true
  sleep 3
  kubectl apply -f "$REG/20-module-nodeSelector.yaml"
  if wait_app vela-system reg-m-ns running 150; then
    sleep 5
    assert_deploy_on_infra "M-NS" "ctrl-kit-provider" vela-system
  else
    bad "M-NS" "parent not running"
  fi

  kubectl delete app -n vela-system reg-m-ns module-ctrl-kit --wait=true 2>/dev/null || true
  sleep 3
  kubectl apply -f "$REG/21-module-nodeAffinity.yaml"
  if wait_app vela-system reg-m-na running 150; then
    sleep 5
    assert_deploy_on_infra "M-NA" "ctrl-kit-provider" vela-system
  else
    bad "M-NA" "parent not running"
  fi
}

run_helmchart() {
  log "== helmchart permutations (reuse / mutate to avoid triple cert-manager) =="
  # H-NS: apply nodeSelector-only onto existing cert-manager release via new app would conflict.
  # Use live-helmchart if present, else apply 30 once.
  if kubectl get app -n vela-system live-helmchart >/dev/null 2>&1; then
    kubectl patch app live-helmchart -n vela-system --type merge -p '{"metadata":{"labels":{"regression.kubevela.io/suite":"addon-scheduling","regression.kubevela.io/case":"H-NS"}}}'
    kubectl get app live-helmchart -n vela-system -o json | python3 -c "
import json,sys
app=json.load(sys.stdin)
app['spec']['components'][0]['traits']=[{'type':'affinity','properties':{'nodeSelector':{'disktype':'ssd','node-type':'infra'}}}]
json.dump(app, sys.stdout)
" | kubectl apply -f -
    wait_app vela-system live-helmchart running 120 || true
    sleep 8
    assert_deploy_on_infra "H-NS" "cert-manager" cert-manager
  else
    kubectl apply -f "$REG/30-helmchart-nodeSelector.yaml"
    wait_app vela-system reg-h-ns running 180 || true
    sleep 10
    assert_deploy_on_infra "H-NS" "cert-manager" reg-cm-ns
  fi

  # H-NA: patch trait to nodeAffinity only
  local target=live-helmchart
  kubectl get app -n vela-system "$target" >/dev/null 2>&1 || target=reg-h-ns
  kubectl get app "$target" -n vela-system -o json | python3 -c '
import json,sys
app=json.load(sys.stdin)
app["metadata"]["labels"]={"regression.kubevela.io/suite":"addon-scheduling","regression.kubevela.io/case":"H-NA"}
app["spec"]["components"][0]["traits"]=[{
  "type":"affinity",
  "properties":{"nodeAffinity":{"required":{"nodeSelectorTerms":[{
    "matchExpressions":[
      {"key":"node-type","values":["infra"]},
      {"key":"disktype","values":["ssd"]}
    ]}]}}}
}]
# clear previous nodeSelector by replacing traits only; also clear values nodeSelector if any
props=app["spec"]["components"][0].get("properties") or {}
vals=props.get("values") or {}
vals.pop("nodeSelector", None)
props["values"]=vals
app["spec"]["components"][0]["properties"]=props
json.dump(app, sys.stdout)
' | kubectl apply -f -
  wait_app vela-system "$target" running 120 || true
  sleep 10
  # After H-NA, nodeSelector may linger from prior revision; require infra placement
  local node
  node=$(kubectl get pods -n cert-manager -l app=cert-manager -o jsonpath='{.items[0].spec.nodeName}' 2>/dev/null || true)
  if [[ "$node" == "$INFRA_NODE" ]]; then
    ok "H-NA (cert-manager on infra)"
  else
    bad "H-NA" "pod on $node"
  fi

  # H-OVER: values want hdd/app, trait wants ssd/infra → trait wins
  kubectl get app "$target" -n vela-system -o json | python3 -c '
import json,sys
app=json.load(sys.stdin)
app["metadata"]["labels"]={"regression.kubevela.io/suite":"addon-scheduling","regression.kubevela.io/case":"H-OVER"}
props=app["spec"]["components"][0]["properties"]
vals=props.get("values") or {}
vals["nodeSelector"]={"disktype":"hdd","node-type":"app"}
props["values"]=vals
app["spec"]["components"][0]["properties"]=props
app["spec"]["components"][0]["traits"]=[{
  "type":"affinity",
  "properties":{"nodeSelector":{"disktype":"ssd","node-type":"infra"}}
}]
json.dump(app, sys.stdout)
' | kubectl apply -f -
  wait_app vela-system "$target" running 120 || true
  sleep 12
  local ns_json
  ns_json=$(kubectl get deploy -n cert-manager cert-manager -o jsonpath='{.spec.template.spec.nodeSelector}' 2>/dev/null || echo '{}')
  if echo "$ns_json" | grep -q '"disktype":"ssd"' && echo "$ns_json" | grep -q '"node-type":"infra"'; then
    ok "H-OVER (trait overlays chart values)"
  else
    bad "H-OVER" "expected trait win, got $ns_json"
  fi
}

run_embed() {
  log "== embedded modules =="
  kubectl delete app -n vela-system reg-embed live-embed-2 addon-kit-suite-inline \
    module-gadget-kit-inline module-widget-kit-inline --wait=false 2>/dev/null || true
  sleep 5
  kubectl apply -f "$REG/40-embed-modules.yaml"
  if wait_app vela-system reg-embed running 180; then
    sleep 8
    # any provider deploy from embedded kits
    local found=0
    while read -r loc; do
      [[ -z "$loc" ]] && continue
      found=1
      assert_deploy_on_infra "E-EMBED" "${loc#*/}" "${loc%/*}"
    done < <(kubectl get deploy -n vela-system -o name | sed 's|deployment.apps/||' | while read -r n; do
      echo "vela-system/$n"
    done | grep -E 'gadget|widget|kit' || true)
    if [[ "$found" -eq 0 ]]; then
      # fallback: check owned module apps have affinity traits
      local child
      for child in $(kubectl get app -n vela-system -o name | sed 's|application.core.oam.dev/||' | grep -E 'module-|addon-kit'); do
        local traits
        traits=$(kubectl get app -n vela-system "$child" -o jsonpath='{.spec.components[*].traits[*].type}' 2>/dev/null || true)
        if echo "$traits" | grep -q affinity; then
          ok "E-EMBED (child $child has affinity trait)"
          found=1
        fi
      done
      [[ "$found" -eq 0 ]] && bad "E-EMBED" "no embedded controller/trait found"
    fi
  else
    bad "E-EMBED" "parent not running"
  fi
}

run_cli_5362() {
  log "== GitHub #5362 CLI --nodeSelector / --scheduling-file =="
  # Make chartmuseum reachable from host
  kubectl port-forward -n default svc/addon-module-chartmuseum 18080:8080 >/tmp/reg-cm-pf.log 2>&1 &
  local PF=$!
  sleep 2
  # Point a host-visible registry (or override via --override / temporary registry)
  "$VELA" addon registry delete local-addons-host >/dev/null 2>&1 || true
  "$VELA" addon registry add local-addons-host --type helm --endpoint http://127.0.0.1:18080 >/dev/null 2>&1 || \
    "$VELA" addon registry add local-addons-host --type=helm --endpoint=http://127.0.0.1:18080 2>&1 || true

  kubectl delete app -n vela-system addon-sched-demo reg-a-combo reg-a-ns reg-a-na --wait=true 2>/dev/null || true
  # disable any prior
  "$VELA" addon disable sched-demo --force 2>/dev/null || true
  sleep 3

  if "$VELA" addon enable local-addons-host/sched-demo \
      --version 1.0.0 \
      --nodeSelector disktype=ssd \
      --nodeSelector node-type=infra \
      -y 2>&1 | tee /tmp/reg-cli-ns.log; then
    sleep 8
    if wait_app vela-system addon-sched-demo running 120; then
      # CLI must have attached affinity trait
      local trait
      trait=$(kubectl get app -n vela-system addon-sched-demo -o jsonpath='{.spec.components[*].traits[*].type}' 2>/dev/null || true)
      if echo "$trait" | grep -q affinity; then
        ok "A-CLI-NS (#5362 trait attached)"
      else
        bad "A-CLI-NS" "no affinity trait on addon app: $trait"
      fi
      assert_deploy_on_infra "A-CLI-NS-place" "sched-demo-ctrl" vela-system
    else
      bad "A-CLI-NS" "addon-sched-demo not running"
    fi
  else
    bad "A-CLI-NS" "vela addon enable failed (see /tmp/reg-cli-ns.log)"
  fi

  "$VELA" addon disable sched-demo --force 2>/dev/null || true
  sleep 5
  if "$VELA" addon enable local-addons-host/sched-demo \
      --version 1.0.0 \
      --scheduling-file "$REG/scheduling-file.yaml" \
      -y 2>&1 | tee /tmp/reg-cli-file.log; then
    sleep 8
    if wait_app vela-system addon-sched-demo running 120; then
      local raw
      raw=$(kubectl get app -n vela-system addon-sched-demo -o jsonpath='{.spec.components[*].traits[*].properties}' 2>/dev/null || true)
      if echo "$raw" | grep -q disktype; then
        ok "A-CLI-FILE (#5362 scheduling-file)"
      else
        bad "A-CLI-FILE" "properties missing disktype: $raw"
      fi
      assert_deploy_on_infra "A-CLI-FILE-place" "sched-demo-ctrl" vela-system
    else
      bad "A-CLI-FILE" "addon not running"
    fi
  else
    bad "A-CLI-FILE" "enable failed (see /tmp/reg-cli-file.log)"
  fi

  kill $PF 2>/dev/null || true
  "$VELA" addon registry delete local-addons-host >/dev/null 2>&1 || true
}

main() {
  log "ROOT=$ROOT KUBECONFIG=$KUBECONFIG"
  kubectl get nodes -L disktype,node-type
  cleanup_reg
  run_webservice
  run_addons
  run_modules
  run_helmchart
  run_embed
  run_cli_5362

  log ""
  log "======== REGRESSION SUMMARY ========"
  for r in "${RESULTS[@]}"; do log "$r"; done
  log "Total PASS=$PASS FAIL=$FAIL"
  [[ "$FAIL" -eq 0 ]]
}

main "$@"
