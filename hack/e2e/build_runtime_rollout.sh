#!/bin/sh

TEMP_DIR="./runtime/rollout/e2e/tmp/"

mkdir -p $TEMP_DIR
cp -r go.mod $TEMP_DIR
cp -r go.sum $TEMP_DIR
cp -r entrypoint.sh $TEMP_DIR
cp -r runtime/rollout/cmd/main.go $TEMP_DIR
cp -r ./apis $TEMP_DIR
cp -r ./pkg $TEMP_DIR
cp -r ./version $TEMP_DIR
