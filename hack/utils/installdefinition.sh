#!/usr/bin/env bash
set -euo pipefail

function install_defs() {
  local def_path="$1"
  if [[ ! -d "$def_path" ]]; then
    echo "Skip: path '$def_path' not found"
    return 0
  fi

  echo "Applying definitions in '$def_path' ..."
  cd "$def_path"

  shopt -s nullglob
  for file in *.yaml; do
    echo "Info: processing $def_path/$file"
    sed -i.bak 's#namespace: {{ include "systemDefinitionNamespace" . }}#namespace: vela-system#g' "$file"
    kubectl apply -f "$file" || { mv "$file.bak" "$file"; return 1; }
    mv "$file.bak" "$file"  # restore original
  done
  shopt -u nullglob

  cd - >/dev/null
}

# Ensure vela-system namespace
if kubectl get namespace vela-system >/dev/null 2>&1; then
  echo "Namespace vela-system exists"
else
  echo "Creating namespace vela-system"
  kubectl create namespace vela-system
fi

install_defs "charts/vela-core/templates/defwithtemplate"
install_defs "charts/vela-core/templates/definitions"
install_defs "charts/vela-core/templates/velaql"
