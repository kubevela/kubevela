#!/usr/bin/env bash

# Copyright 2022 The KubeVela Authors.
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

set -o errexit

# This script helps to sync SDK to other repos.

VELA_GO_SDK=kubevela-contrib/kubevela-go-sdk

if [[ -n "$SSH_DEPLOY_KEY" ]]; then
  mkdir -p ~/.ssh
  echo "$SSH_DEPLOY_KEY" >~/.ssh/id_rsa
  chmod 600 ~/.ssh/id_rsa
fi

cd ..

config() {
  git config --global user.email "kubevela.bot@aliyun.com"
  git config --global user.name "kubevela-bot"
}

cloneAndClearCoreAPI() {
  echo "git clone"

  if [[ -n "$SSH_DEPLOY_KEY" ]]; then
    git clone --single-branch --depth 1 git@github.com:$VELA_GO_SDK.git kubevela-go-sdk
  else
    git clone --single-branch --depth 1 https://github.com/$VELA_GO_SDK.git kubevela-go-sdk
  fi

  echo "Clear kubevela-go-sdk pkg/apis/common, pkg/apis/component, pkg/apis/policy, pkg/apis/trait, pkg/apis/workflow-step, pkg/apis/utils, pkg/apis/types.go "
  rm -rf kubevela-go-sdk/pkg/apis/common
  rm -rf kubevela-go-sdk/pkg/apis/component
  rm -rf kubevela-go-sdk/pkg/apis/policy
  rm -rf kubevela-go-sdk/pkg/apis/trait
  rm -rf kubevela-go-sdk/pkg/apis/workflow-step
  rm -rf kubevela-go-sdk/pkg/apis/utils
}

updateRepo() {
  cd kubevela
  bin/vela def gen-api -f vela-templates/definitions/internal/ -o ../kubevela-go-sdk --package=github.com/$VELA_GO_SDK --init
}

syncRepo() {
  cd ../kubevela-go-sdk
  go mod tidy
  echo "Push to $VELA_GO_SDK"
  if git diff --quiet; then
    echo "no changes, skip pushing commit"
  else
    git add .
    git commit -m "Generated from kubevela-$VERSION from commit $COMMIT_ID"
    git push origin main
  fi

  # push new tag anyway
  # Only tags if VERSION starts with refs/tags/, remove the prefix and push it
  if [[ "$VERSION" == refs/tags/* ]]; then
    VERSION=${VERSION#refs/tags/}
  else
    echo "VERSION $VERSION is not a tag, skip pushing tag"
    return
  fi

  echo "push tag $VERSION"
  git tag "$VERSION"
  git push origin "$VERSION"
}

main() {
  config
  cloneAndClearCoreAPI
  updateRepo
  syncRepo
}

main
