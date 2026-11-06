#!/usr/bin/env bash
# Exercise worker resolution without depending on the host's CPU count.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
export TEST_NPROC_OUTPUT=6 TEST_NPROC_STATUS=0 TEST_GETCONF_OUTPUT=4
nproc() {
  printf '%s\n' "${TEST_NPROC_OUTPUT-6}"
  return "${TEST_NPROC_STATUS-0}"
}
getconf() {
  [[ "$1" == _NPROCESSORS_ONLN ]] || return 1
  printf '%s\n' "${TEST_GETCONF_OUTPUT-4}"
}
export -f nproc getconf

expect_workers() {
  local expected=$1 actual
  shift
  actual=$(bash "$script_dir/ginkgo_workers.sh" "$@")
  [[ "$actual" == "$expected" ]] || {
    echo "Expected $expected workers, got '$actual' for arguments: $*" >&2
    exit 1
  }
}

expect_invalid() {
  local actual
  if actual=$(bash "$script_dir/ginkgo_workers.sh" "$1" 2>&1); then
    echo "Expected worker setting '$1' to fail, got '$actual'" >&2
    exit 1
  fi
  [[ "$actual" == *"Ginkgo workers must be 'auto' or a positive integer"* ]] || {
    echo "Missing validation error for '$1': $actual" >&2
    exit 1
  }
}

expect_workers 6
expect_workers 6 auto
expect_workers 1 1
expect_workers 12 12
TEST_NPROC_STATUS=1 expect_workers 4 auto
for invalid in '' 0 -1 1.5 ' 2' 02 AUTO; do
  expect_invalid "$invalid"
done
TEST_NPROC_OUTPUT=0 expect_invalid auto
TEST_NPROC_STATUS=1 TEST_GETCONF_OUTPUT=0 expect_invalid auto
echo 'Ginkgo worker resolution verified'
