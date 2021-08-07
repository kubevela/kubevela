#!/usr/bin/env bash

set -e

LIGHTGRAY='\033[0;37m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

HEAD_PROMPT="${LIGHTGRAY}[${0}]${NC} "

SCRIPT_DIR=$(dirname "$0")
pushd "$SCRIPT_DIR" &> /dev/null

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

echo -e "${HEAD_PROMPT}Start generating definitions at ${LIGHTGRAY}${SCRIPT_DIR}${NC} ..."
export IGNORE_KUBE_CONFIG=true
echo -ne "${HEAD_PROMPT}${YELLOW}(0/2) Generating internal definitions from ${LIGHTGRAY}${INTERNAL_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${INTERNAL_TEMPLATE_DIR}${YELLOW} ... "
export AS_HELM_CHART=true
render $INTERNAL_DEFINITION_DIR $INTERNAL_TEMPLATE_DIR
echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${YELLOW}(1/2) Generating registry definitions from ${LIGHTGRAY}${REGISTRY_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${REGISTRY_TEMPLATE_DIR}${YELLOW} ... "
export AS_HELM_CHART=system
render $REGISTRY_DEFINITION_DIR $REGISTRY_TEMPLATE_DIR
echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${GREEN}(2/2) All done.${NC}\n"
popd &> /dev/null
