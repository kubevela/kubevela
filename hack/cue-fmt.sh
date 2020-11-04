#!/usr/bin/env bash

set -e

if ! cue version ; then
  echo "Installing CUE..."
  GO111MODULE=off go get -u cuelang.org/go/cmd/cue
fi

echo "Formatting CUE templates..."
cue fmt ./hack/vela-templates/cue/* 
