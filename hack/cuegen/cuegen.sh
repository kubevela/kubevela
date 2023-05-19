#!/usr/bin/env bash

set -e

# Usage: cuegen.sh dir [flagsToVela]
# Example: cuegen.sh references/cuegen/generators/provider/testdata/ \
#           -t provider \
#           --types *k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured=ellipsis


# check vela binary
if ! [ -x "$(command -v vela)" ]; then
  echo "Please put vela binary in the PATH"
  exit 1
fi

# get dir from first arg
dir="$1"
if [ -z "$dir" ]; then
  echo "Please provide a directory"
  exit 1
fi

echo "Generating CUE files from go files in $dir"
echo "========"

find "$dir" -name "*.go" | while read -r file; do
  echo "- Generating CUE file for $file"

  # redirect output if command exits successfully
  (out=$(vela def gen-cue ${*:2} "$file") && echo "$out" > "${file%.go}".cue) &
done

echo "========"
echo "Waiting for all background processes to finish"

wait
sleep 5s # wait for all stderr to be printed

echo "Done"
