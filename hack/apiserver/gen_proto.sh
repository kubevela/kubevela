#!/usr/bin/env bash

PROTOCMD="protoc -I . \
		--go_out=. --go_opt=paths=source_relative \
		"

for entry in "pkg/apiserver/proto/model"/*
do
  if [ "${entry##*.}" = "proto" ]; then
    eval $PROTOCMD "${entry}"
  fi
done
