#!/usr/bin/env bash

# This script fixes vendoring issues in the repository

set -e

echo "==> Cleaning up go.mod and go.sum"
go mod tidy

echo "==> Verifying modules"
go mod verify

echo "==> Regenerating vendor directory"
rm -rf vendor
go mod vendor

echo "==> Verifying build with vendor mode"
go build -mod=vendor ./...

echo "==> Vendor directory has been successfully updated"
