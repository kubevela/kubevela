#!/usr/bin/env bash

set -e

SCRIPT_DIR=$(dirname "$0")
pushd $SCRIPT_DIR

TEMPLATE_DIR="../../charts/vela-core/templates/defwithtemplate"

rm tmp* 2>/dev/null | true
rm ${TEMPLATE_DIR}/* 2>/dev/null | true

for filename in `ls cue`; do
  cat "cue/${filename}" > tmp
  echo "" >> tmp
  sed -i.bak 's/^/      /' tmp

  nameonly="${filename%.*}"

  cp "definitions/${nameonly}.yaml" "${TEMPLATE_DIR}/${nameonly}.yaml"
  cat tmp >> "${TEMPLATE_DIR}/${nameonly}.yaml"
done

rm tmp*

echo "done"

popd
