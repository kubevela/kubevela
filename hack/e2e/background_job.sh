#!/usr/bin/env bash
# Run a command in the background in one workflow step and collect its result
# in a later one.
# Usage: background_job.sh start <name> <command> [args...]
#        background_job.sh wait <name> [timeout-seconds]
# State lives in BACKGROUND_JOB_DIR (default: $RUNNER_TEMP or $TMPDIR); wait
# polls every BACKGROUND_JOB_POLL seconds (default 5).
set -euo pipefail

state_dir=${BACKGROUND_JOB_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/background-jobs}
poll=${BACKGROUND_JOB_POLL:-5}
action=${1:?start or wait required}
name=${2:?job name required}
shift 2
log="${state_dir}/${name}.log"
rc="${state_dir}/${name}.rc"

case "${action}" in
  start)
    (($# > 0)) || { echo "background_job.sh start ${name}: command required" >&2; exit 2; }
    mkdir -p "${state_dir}"
    rm -f "${log}" "${rc}"
    # Detached from the step's stdout and stderr, which the runner waits on.
    # shellcheck disable=SC2016 # expanded by the inner shell
    nohup bash -c '"$@" >"$0" 2>&1; echo $? >"$0.rc.tmp"; mv "$0.rc.tmp" "${0%.log}.rc"' \
      "${log}" "$@" </dev/null >/dev/null 2>&1 &
    echo "Started ${name} in the background (log: ${log})"
    ;;
  wait)
    timeout=${1:-1800}
    deadline=$((SECONDS + timeout))
    until [[ -f "${rc}" ]]; do
      if ((SECONDS >= deadline)); then
        [[ -f "${log}" ]] && cat "${log}"
        echo "Timed out after ${timeout}s waiting for ${name}" >&2
        exit 1
      fi
      sleep "${poll}"
    done
    cat "${log}"
    status=$(<"${rc}")
    if [[ "${status}" != 0 ]]; then
      echo "${name} failed with exit code ${status}" >&2
    fi
    exit "${status}"
    ;;
  *)
    echo "unknown action ${action}; want start or wait" >&2
    exit 2
    ;;
esac
