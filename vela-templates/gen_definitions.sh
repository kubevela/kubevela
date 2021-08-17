#!/usr/bin/env bash

set -e
export IGNORE_KUBE_CONFIG=true

LIGHTGRAY='\033[0;37m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

HEAD_PROMPT="${LIGHTGRAY}[${0}]${NC} "

SCRIPT_DIR=$(dirname "$0")
pushd "$SCRIPT_DIR" &> /dev/null

INTERNAL_DEFINITION_DIR="definitions/internal"
REGISTRY_DEFINITION_DIR="definitions/registry"
INTERNAL_TEMPLATE_DIR="../charts/vela-core/templates/defwithtemplate"
REGISTRY_TEMPLATE_DIR="registry/auto-gen"

VELA_CMD="../bin/vela"
if [ ! -f "$VELA_CMD" ]; then
  echo -e "${HEAD_PROMPT}${YELLOW}Failed to get vela command, fallback to use \`go run\`."
  VELA_CMD="go run ../references/cmd/cli/main.go"
else
  echo -e "${HEAD_PROMPT}${GREEN}Got vela command binary."
  $VELA_CMD version
fi

function render {
  inputDir=$1
  outputDir=$2
  rm "$outputDir"/* 2>/dev/null || true
  mkdir -p "$outputDir"
  $VELA_CMD def render "$inputDir" -o "$outputDir" --message "Definition source cue file: vela-templates/$inputDir/{{INPUT_FILENAME}}"
  retVal=$?
  if [ $retVal -ne 0 ]; then
    echo -ne "${RED}Failed. Exit code: ${retVal}.${NC}\n"
    exit $retVal
  fi
}

echo -e "${HEAD_PROMPT}Start generating definitions at ${LIGHTGRAY}${SCRIPT_DIR}${NC} ..."
echo -ne "${HEAD_PROMPT}${YELLOW}(0/2) Generating internal definitions from ${LIGHTGRAY}${INTERNAL_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${INTERNAL_TEMPLATE_DIR}${YELLOW} ... "
export AS_HELM_CHART=true
render $INTERNAL_DEFINITION_DIR $INTERNAL_TEMPLATE_DIR
echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${YELLOW}(1/2) Generating registry definitions from ${LIGHTGRAY}${REGISTRY_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${REGISTRY_TEMPLATE_DIR}${YELLOW} ... "
export AS_HELM_CHART=system
render $REGISTRY_DEFINITION_DIR $REGISTRY_TEMPLATE_DIR
echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${GREEN}(2/2) All done.${NC}\n"
popd &> /dev/null
