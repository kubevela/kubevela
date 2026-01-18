#!/bin/bash -l
#
# Copyright 2021. The KubeVela Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -ex

cd "$(dirname "${BASH_SOURCE[0]}")/../.."

WORK_TEMP_DIR="./clientgen_work_temp"

# client generator parameters
CODEGEN_GENERATORS="all" # deepcopy,defaulter,client,lister,informer or all
OUTPUT_PACKAGE="github.com/oam-dev/kubevela/pkg/generated/client"
APIS_PACKAGE="github.com/oam-dev/kubevela/apis"
CODEGEN_GROUP_VERSIONS="core.oam.dev:v1beta1"
OUTPUT_DIR="${WORK_TEMP_DIR}"
BOILERPLATE_FILE="./hack/boilerplate.go.txt"

installDep() {
  cp go.mod go.sum "${WORK_TEMP_DIR}/backup/"

  cat <<EOF >"${WORK_TEMP_DIR}/tools.go"
// +build tools

package tools

import _ "k8s.io/code-generator"
EOF
  go get github.com/oam-dev/kubevela/clientgen_work_temp
  go mod vendor
}

clientGen() {
  # Using kube_codegen.sh as generate-groups.sh has been removed
  chmod +x ./vendor/k8s.io/code-generator/kube_codegen.sh 
  chmod +x ./vendor/k8s.io/code-generator/generate-internal-groups.sh
  
  # kube_codegen.sh uses different syntax - call it via the kube_codegen module
  source ./vendor/k8s.io/code-generator/kube_codegen.sh
  
  # Generate code using the new API
  kube::codegen::gen_client \
    --with-watch \
    --output-dir "${OUTPUT_DIR}" \
    --output-pkg "${OUTPUT_PACKAGE}" \
    --boilerplate "${BOILERPLATE_FILE}" \
    "./apis"

  rm -rf ./pkg/generated/
  mkdir -p ./pkg/generated/
  
  # Move the generated client code
  if [ -d "${WORK_TEMP_DIR}/clientset" ] || [ -d "${WORK_TEMP_DIR}/informers" ] || [ -d "${WORK_TEMP_DIR}/listers" ]; then
    mkdir -p ./pkg/generated/client
    if [ -d "${WORK_TEMP_DIR}/clientset" ]; then
      mv "${WORK_TEMP_DIR}/clientset" ./pkg/generated/client/
    fi
    if [ -d "${WORK_TEMP_DIR}/informers" ]; then
      mv "${WORK_TEMP_DIR}/informers" ./pkg/generated/client/
    fi
    if [ -d "${WORK_TEMP_DIR}/listers" ]; then
      mv "${WORK_TEMP_DIR}/listers" ./pkg/generated/client/
    fi
  else
    echo "Warning: Generated client directories not found"
    find "${WORK_TEMP_DIR}" -type d 2>/dev/null || true
  fi
}

EXPECTED_CONTROLLER_GEN_VERSION=v0.16.5
CONTROLLER_GEN="$(go env GOPATH)"/bin/controller-gen

deepcopyGen() {
  echo "generating zz_generated.deepcopy"
  if ! command -v "$CONTROLLER_GEN" &> /dev/null; then
    VERSION="NOT_INSTALLED"
  else
    VERSION=$($CONTROLLER_GEN --version)
  fi
  if [ "$VERSION" == "Version: ${EXPECTED_CONTROLLER_GEN_VERSION}" ]; then
    echo "controller-gen is already installed"
    :
  else
    echo "installing new controller-gen: ${VERSION}=>${EXPECTED_CONTROLLER_GEN_VERSION}"
    rm -rf "$CONTROLLER_GEN"
    GOBIN="$(go env GOPATH)"/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@${EXPECTED_CONTROLLER_GEN_VERSION}
  fi
  $CONTROLLER_GEN object:headerFile="./hack/boilerplate.go.txt" paths="./apis/..."
}

cleanup() {
  mv "${WORK_TEMP_DIR}/backup/"* ./
  rm -drf "${WORK_TEMP_DIR:?}/"
  rm -drf vendor
}

main() {
  mkdir -p "${WORK_TEMP_DIR}/backup/"
  installDep
  clientGen
  cleanup
}

main
deepcopyGen
