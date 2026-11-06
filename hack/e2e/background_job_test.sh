#!/usr/bin/env bash
# Exercise background_job.sh start/wait across separate invocations.
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/background_job.sh"
BACKGROUND_JOB_DIR=$(mktemp -d "${TMPDIR:-/tmp}/background-job-test.XXXXXX")
export BACKGROUND_JOB_DIR BACKGROUND_JOB_POLL=0.2
trap 'rm -rf "$BACKGROUND_JOB_DIR"' EXIT

fail() {
  echo "background_job_test: $*" >&2
  exit 1
}

bash "$script" start ok bash -c 'sleep 1; echo "hello from ok"' >/dev/null
out=$(bash "$script" wait ok 30) || fail "wait ok returned non-zero"
[[ "$out" == *"hello from ok"* ]] || fail "wait ok did not print the job log: $out"

bash "$script" start broken bash -c 'echo "about to fail"; exit 3' >/dev/null
set +e
out=$(bash "$script" wait broken 30 2>&1)
status=$?
set -e
((status == 3)) || fail "wait broken returned $status, want 3"
[[ "$out" == *"about to fail"* ]] || fail "wait broken did not print the job log: $out"

bash "$script" start slow sleep 3 >/dev/null
set +e
bash "$script" wait slow 1 >/dev/null 2>&1
status=$?
set -e
((status == 1)) || fail "wait slow returned $status, want a timeout (1)"

set +e
bash "$script" start empty >/dev/null 2>&1
status=$?
set -e
((status == 2)) || fail "start without a command returned $status, want 2"

echo "background_job.sh tests passed"
