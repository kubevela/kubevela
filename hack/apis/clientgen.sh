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

set -e

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
  bash ./vendor/k8s.io/code-generator/generate-groups.sh "${CODEGEN_GENERATORS}" \
    ${OUTPUT_PACKAGE} \
    ${APIS_PACKAGE} \
    "${CODEGEN_GROUP_VERSIONS}" \
    --output-base "${OUTPUT_DIR}" \
    --go-header-file "${BOILERPLATE_FILE}"

  mkdir -p ./pkg/generated/
  mv "${WORK_TEMP_DIR}/github.com/oam-dev/kubevela/pkg/generated/client" ./pkg/generated/
}

cleanup() {
  mv "${WORK_TEMP_DIR}/backup/"* ./
  rm -drf "${WORK_TEMP_DIR}/"
  rm -drf vendor
}

main() {
  mkdir -p "${WORK_TEMP_DIR}/backup/"
  installDep
  clientGen
  cleanup
}

main
