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

if [[ -n "$SSH_DEPLOY_KEY" ]]
then
  mkdir -p ~/.ssh
  echo "$SSH_DEPLOY_KEY" > ~/.ssh/id_rsa
  chmod 600 ~/.ssh/id_rsa
fi

echo "git clone"
git config --global user.email "kubevela.bot@aliyun.com"
git config --global user.name "kubevela-bot"
git clone --single-branch --depth 1 git@github.com:oam-dev/kubevela.io.git git-page

echo "sidebars updates"
cat docs/sidebars.js > git-page/sidebars.js

echo "clear en docs"
rm -r git-page/docs/*
#echo "clear zh docs"
#rm -r git-page/i18n/zh/docusaurus-plugin-content-docs/*

echo "update docs"
cp -R docs/en/* git-page/docs/
#cp -R docs/zh-CN/* git-page/i18n/zh/docusaurus-plugin-content-docs/

echo "git push"
cd git-page

# Check for release created only
SUB='v'
if [[ "$VERSION" == "$SUB"* ]]
then
  # vx.y.z -> vx.y
  sub="."
  major="$(cut -d"$sub" -f1 <<<"$VERSION")"
  minor="$(cut -d"$sub" -f2 <<<"$VERSION")"

  version=${major}.${minor}
  echo "updating website for version $version"

  if grep -q $version versions.json; then
    rm -r versioned_docs/version-${version}/
    rm versioned_sidebars/version-${version}-sidebars.json
    sed -i.bak "/${version}/d" versions.json
    rm versions.json.bak
  fi

  yarn add nodejieba
  if [ -e yarn.lock ]; then
  yarn install --frozen-lockfile
  elif [ -e package-lock.json ]; then
  npm ci
  else
  npm i
  fi

  yarn run docusaurus docs:version $version
fi

# Check for release branch update only
SUB='release-'
if [[ "$VERSION" == *"$SUB"* ]]
then

  # release-x.y -> vx.y
  version=$(echo $VERSION|sed -e 's/\/*.*\/*-/v/g')
  echo "updating website for version $version"

  if grep -q $version versions.json; then
    rm -r versioned_docs/version-${version}/
    rm versioned_sidebars/version-${version}-sidebars.json
    sed -i.bak "/${version}/d" versions.json
    rm versions.json.bak
  fi

  yarn add nodejieba
  if [ -e yarn.lock ]; then
  yarn install --frozen-lockfile
  elif [ -e package-lock.json ]; then
  npm ci
  else
  npm i
  fi

  yarn run docusaurus docs:version $version
fi



if git diff --quiet
then
  echo "nothing need to push, finished!"
else
  git add .
  git commit -m "sync commit $COMMIT_ID from kubevela-$VERSION"
  git push origin main
fi