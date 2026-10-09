#!/usr/bin/env bash
# Resolve the same worker setting for CI and local Make targets.
set -euo pipefail

requested=${1:-auto}
if [[ "$requested" == auto ]]; then
  if ! workers=$(nproc 2>/dev/null); then
    workers=$(getconf _NPROCESSORS_ONLN)
  fi
else
  workers=$requested
fi

if [[ ! "$workers" =~ ^[1-9][0-9]*$ ]]; then
  echo "Ginkgo workers must be 'auto' or a positive integer; got '$requested' (resolved '$workers')" >&2
  exit 1
fi
printf '%s\n' "$workers"
