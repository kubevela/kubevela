#!/usr/bin/env bash

set -e

SCRIPT_DIR=$(dirname "$0")
pushd "$SCRIPT_DIR"

INTERNAL_DEFINITION_DIR="definitions/internal"
REGISTRY_DEFINITION_DIR="definitions/registry"
INTERNAL_TEMPLATE_DIR="../charts/vela-core/templates/defwithtemplate"
REGISTRY_TEMPLATE_DIR="registry/auto-gen"

function render {
  inputDir=$1
  outputDir=$2
  rm "$outputDir"/* 2>/dev/null || true
  mkdir -p "$outputDir"
  go run ../references/cmd/cli/main.go def render "$inputDir" -o "$outputDir"
}

export AS_HELM_CHART=true
render $INTERNAL_DEFINITION_DIR $INTERNAL_TEMPLATE_DIR
echo "done generate internal definitions"
export AS_HELM_CHART=system
render $REGISTRY_DEFINITION_DIR $REGISTRY_TEMPLATE_DIR
echo "done generate registry definitions"
echo "all done"
popd
