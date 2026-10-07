#!/bin/sh
# Runs before an upgrade. Releases from before the builtin definitions moved out
# of the release manifest own them, so Helm would delete them on the way past
# this upgrade, and their DefinitionRevisions with them. Helm skips deleting a
# resource whose live copy carries helm.sh/resource-policy=keep, so mark the
# builtins this release owns.
#
# Env: DEFINITION_NAMESPACE, RELEASE_NAME, BUILTIN_DEFINITIONS (resource/name, space separated)
set -eu

served=$(kubectl api-resources --api-group=core.oam.dev -o name)

for ref in $BUILTIN_DEFINITIONS; do
  resource=${ref%%/*}
  if ! printf '%s\n' "$served" | grep -qx "$resource"; then
    continue
  fi

  # "<owning release> <resource-policy>"; only owned and not yet kept is changed.
  state=$(kubectl -n "$DEFINITION_NAMESPACE" get "$ref" --ignore-not-found \
    -o jsonpath='{.metadata.annotations.meta\.helm\.sh/release-name} {.metadata.annotations.helm\.sh/resource-policy}')
  if [ "$state" = "$RELEASE_NAME " ]; then
    kubectl -n "$DEFINITION_NAMESPACE" annotate "$ref" helm.sh/resource-policy=keep
  fi
done
