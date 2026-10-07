#!/bin/sh
# Runs after install, upgrade and rollback. Waits for this release's controller
# so the webhook validating the builtin definitions is the new one, not the pod
# being replaced, then applies them from the mounted ConfigMaps.
#
# Env: CONTROLLER_NAMESPACE, CONTROLLER_DEPLOYMENT, ROLLOUT_TIMEOUT_SECONDS,
#      APPLY_RETRIES, WEBHOOK_CONFIGURATION (empty when admission webhooks are off)
set -eu

kubectl -n "$CONTROLLER_NAMESPACE" rollout status "deployment/$CONTROLLER_DEPLOYMENT" \
  --timeout="${ROLLOUT_TIMEOUT_SECONDS}s"

# With cert-manager no hook patches the CA bundle in; its CA injector does, on
# its own schedule. Until every webhook has one the API server cannot call them.
if [ -n "$WEBHOOK_CONFIGURATION" ]; then
  waited=0
  while kubectl get validatingwebhookconfiguration "$WEBHOOK_CONFIGURATION" \
    -o jsonpath='{range .webhooks[*]}{.name}={.clientConfig.caBundle}{"\n"}{end}' | grep -q '=$'; do
    if [ "$waited" -ge "$ROLLOUT_TIMEOUT_SECONDS" ]; then
      echo "Webhook $WEBHOOK_CONFIGURATION still has no CA bundle after ${waited}s" >&2
      exit 1
    fi
    sleep 5
    waited=$((waited + 5))
  done
fi

# A webhook call can still fail transiently right after the rollout, so a
# failed apply is retried.
attempt=1
until kubectl apply --server-side --force-conflicts \
  --field-manager=vela-core-builtin-definitions -f /definitions; do
  if [ "$attempt" -ge "$APPLY_RETRIES" ]; then
    echo "Giving up after $attempt attempts" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 10
done
