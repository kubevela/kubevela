#!/usr/bin/env bash

set -e

echo "building binary"
echo "========"
go build -o bin/vela ./cmd/vela/

echo "vela up"
echo "========"
cd testapp
../bin/vela up
cd ..

echo "cat deploy yaml"
echo "========"
cat testapp/.vela/deploy.yaml
