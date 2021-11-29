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

DEPRECATED_DEFINITION_DIR="definitions/deprecated"
INTERNAL_DEFINITION_DIR="definitions/internal"
REGISTRY_DEFINITION_DIR="definitions/registry"
INTERNAL_TEMPLATE_DIR="../charts/vela-core/templates/defwithtemplate"
MINIMAL_TEMPLATE_DIR="../charts/vela-minimal/templates"
REGISTRY_TEMPLATE_DIR="registry/auto-gen"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=""
case $(uname -m) in
    i386)   ARCH="386" ;;
    i686)   ARCH="386" ;;
    x86_64) ARCH="amd64" ;;
    arm)    ARCH="arm64" ;;
esac

VELA_CMD="../bin/vela"
if [ ! -f "$VELA_CMD" ]; then
  VELA_CMD="../_bin/vela/$OS-$ARCH/vela"
  echo -e "${HEAD_PROMPT}${LIGHTGRAY}Search cross build vela binary in ${VELA_CMD}.${NC}"
fi
if [ ! -f "$VELA_CMD" ]; then
    echo -e "${HEAD_PROMPT}${YELLOW}Failed to get vela command, fallback to use \`go run\`.${NC}"
    VELA_CMD="go run ../references/cmd/cli/main.go"
else
  echo -e "${HEAD_PROMPT}${GREEN}Got vela command binary, version information:"
  $VELA_CMD version
fi

function render {
  inputDir=$1
  outputDir=$2
  if [ -z "$3" ] || [ "$3" != "--append" ]; then
    rm "$outputDir"/* 2>/dev/null || true
  fi
  mkdir -p "$outputDir"
  $VELA_CMD def render "$inputDir" -o "$outputDir" --message "Definition source cue file: vela-templates/$inputDir/{{INPUT_FILENAME}}"
  retVal=$?
  if [ $retVal -ne 0 ]; then
    echo -ne "${RED}Failed. Exit code: ${retVal}.${NC}\n"
    exit $retVal
  fi
}

function renderMinimal {
  inputDir=$1
  outputDir=$2

  cp -r "$inputDir" "$outputDir"

  rm -f "$outputDir"/defwithtemplate/env-binding.yaml
  rm -f "$outputDir"/defwithtemplate/deploy2env.yaml
}

echo -e "${HEAD_PROMPT}Start generating definitions at ${LIGHTGRAY}${SCRIPT_DIR}${NC} ..."

# Generate Internal definitions
echo -ne "${HEAD_PROMPT}${YELLOW}(0/3) Generating internal definitions from ${LIGHTGRAY}${INTERNAL_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${INTERNAL_TEMPLATE_DIR}${YELLOW} ... "
export AS_HELM_CHART=true
render $INTERNAL_DEFINITION_DIR $INTERNAL_TEMPLATE_DIR
renderMinimal $INTERNAL_TEMPLATE_DIR $MINIMAL_TEMPLATE_DIR

# Generate deprecated definitions
echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${YELLOW}(1/3) Generating deprecated definitions from ${LIGHTGRAY}${DEPRECATED_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${INTERNAL_TEMPLATE_DIR}${YELLOW} ... "
render $DEPRECATED_DEFINITION_DIR $INTERNAL_TEMPLATE_DIR --append
echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${YELLOW}(2/3) Generating registry definitions from ${LIGHTGRAY}${REGISTRY_DEFINITION_DIR}${YELLOW} to ${LIGHTGRAY}${REGISTRY_TEMPLATE_DIR}${YELLOW} ... "

# Generate registry definitions
export AS_HELM_CHART=system
render $REGISTRY_DEFINITION_DIR $REGISTRY_TEMPLATE_DIR

echo -ne "${GREEN}Generated.\n${HEAD_PROMPT}${GREEN}(3/3) All done.${NC}\n"
popd &> /dev/null
