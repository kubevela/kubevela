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
git config --global user.email "kubevela.bot@aliyun.com"
git config --global user.name "kubevela-bot"

echo "update kubevela.io"
cd kubevela.io
git config core.sparsecheckout true
git remote add origin https://github.com/oam-dev/kubevela.io.git
echo "*" >> .git/info/sparse-checkout
echo "!/docs/**" >> .git/info/sparse-checkout
echo "!/sidebars.js" >> .git/info/sparse-checkout
git pull --depth 1 origin main 

while getopts "t:" arg 
do
        case $arg in
             t)
                if [ $OPTARG == "start" ];
                then
                    echo "start docs"
                    yarn install && yarn start --host 0.0.0.0 --port 3000
                fi

                if [ $OPTARG == "build" ];
                then
                    echo "build docs"
                    yarn install && yarn run build
                fi
                ;;                
             ?) 
                echo "unkonw argument"
                exit 1
            ;;
        esac
done
