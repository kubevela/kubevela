#!/usr/bin/env bash

set -e

pushd hack/appfile

rm tmp* 2>/dev/null | true
rm deploy/* 2>/dev/null | true
mkdir deploy 2>/dev/null | true

for filename in `ls cue-templates`; do
  cat cue-templates/$filename > tmp
  echo "" >> tmp
  sed -i.bak 's/^/      /' tmp

  fname="${filename%.*}"

  cp definitions/${fname}.yaml deploy/${fname}.yaml
  cat tmp >> deploy/${fname}.yaml
done

rm tmp*

echo "done"

popd
