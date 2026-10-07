#!/bin/sh
# Runs after install, upgrade and rollback. Waits for this release's controller
# so the webhook validating the builtin definitions is the new one, not the pod
# being replaced, then applies them from the mounted ConfigMaps.
#
# Env: CONTROLLER_NAMESPACE, CONTROLLER_DEPLOYMENT, ROLLOUT_TIMEOUT_SECONDS, APPLY_RETRIES
set -eu

kubectl -n "$CONTROLLER_NAMESPACE" rollout status "deployment/$CONTROLLER_DEPLOYMENT" \
  --timeout="${ROLLOUT_TIMEOUT_SECONDS}s"

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
