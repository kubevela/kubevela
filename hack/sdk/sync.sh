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

git config --global user.email "kubevela.bot@aliyun.com"
git config --global user.name "kubevela-bot"

cloneAndClearRepo() {
  echo "git clone"

  if [[ -n "$SSH_DEPLOY_KEY" ]]; then
    git clone --single-branch --depth 1 git@github.com:$VELA_GO_SDK.git kubevela-go-sdk
  else
    git clone --single-branch --depth 1 https://github.com/$VELA_GO_SDK.git kubevela-go-sdk
  fi

  echo "clear kubevela-go-sdk pkg/*"
  rm -r kubevela-go-sdk/pkg/*
}

updateRepo() {
  bin/vela def gen-api -f vela-templates/definitions/internal/ -o ./kubevela-go-sdk --package=github.com/$VELA_GO_SDK
}

syncRepo() {
  cd kubevela-go-sdk
  echo "push to $VELA_GO_SDK"
  if git diff --quiet; then
    echo "nothing need to push, finished!"
  else
    git add .
    git commit -m "Generated from kubevela-$VERSION from commit $COMMIT_ID"
    git tag "$VERSION"
    git push origin main
    git push origin "$VERSION"
  fi
}

main() {
  cloneAndClearRepo
  updateRepo
  syncRepo
}

main $1
