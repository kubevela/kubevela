#!/usr/bin/env bash

set -e

echo "building binary"
echo "========"
go build -o bin/vela ./references/cmd/cli/
export PATH=bin/:$PATH

echo "vela up"
echo "========"
cd examples/testapp
vela up

cd ../..
