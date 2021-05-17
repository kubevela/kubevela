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

if [ ! -d "./git-page" ]; then
  git clone --single-branch --depth 1 https://github.com/oam-dev/kubevela.io.git git-page
fi

rm -r git-page/docs
rm git-page/sidebars.js
cat docs/sidebars.js > git-page/sidebars.js
cp -R docs/en git-page/docs
cd git-page && yarn install && yarn start