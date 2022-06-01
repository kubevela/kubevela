#!/usr/bin/env bash

set -e

SCRIPT_DIR=$(dirname "$0")
pushd "$SCRIPT_DIR"

TEMPLATE_DIR="../../config/crd/base"

echo "clean up unused fields of CRDs"

for filename in `ls "$TEMPLATE_DIR"`; do

  sed -i.bak '/creationTimestamp: null/d' "${TEMPLATE_DIR}/$filename"

done

rm ${TEMPLATE_DIR}/*.bak

TEMPLATE_DIR="../../legacy/charts/vela-core-legacy/crds"

for filename in `ls "$TEMPLATE_DIR"`; do

  sed -i.bak '/creationTimestamp: null/d' "${TEMPLATE_DIR}/$filename"

done

rm ${TEMPLATE_DIR}/*.bak


popd
