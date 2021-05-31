#!/usr/bin/env bash

set -e

SCRIPT_DIR=$(dirname "$0")
pushd $SCRIPT_DIR

INTERNAL_TEMPLATE_DIR="../../charts/vela-core/templates/defwithtemplate"
REGISTRY_TEMPLATE_DIR="../../registry"

rm ${INTERNAL_TEMPLATE_DIR}/* 2>/dev/null | true
rm ${REGISTRY_TEMPLATE_DIR}/* 2>/dev/null | true


for filename in internal/cue/*; do
  filename=$(basename "${filename}")
  nameonly="${filename%.*}"

  sh ./mergedef.sh "internal/definitions/${nameonly}.yaml" "internal/cue/${nameonly}.cue" > "${INTERNAL_TEMPLATE_DIR}/${nameonly}.yaml"
done

echo "done generate internal definitions"

for filename in registry/cue/*; do
  filename=$(basename "${filename}")
  nameonly="${filename%.*}"

  sh ./mergedef.sh "registry/definitions/${nameonly}.yaml" "registry/cue/${nameonly}.cue" > "${REGISTRY_TEMPLATE_DIR}/${nameonly}.yaml"
done

echo "done generate registry definitions"

echo "all done"

popd
