#!/usr/bin/env bash

set -e

echo "building binary"
echo "========"
go build -o bin/vela ./cmd/vela/
export PATH=bin/:$PATH

echo "vela up"
echo "========"
cd examples/testapp
vela up

echo "cat deploy yaml"
echo "========"
cat .vela/deploy.yaml

cd ../..
