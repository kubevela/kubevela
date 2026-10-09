#!/usr/bin/env bash
# Wait until another job of this workflow run has uploaded an artifact.
# Usage: wait_for_artifact.sh <artifact-name> <producing-job-name> [timeout-seconds]
# Needs GH_TOKEN with actions: read, and the GITHUB_* variables of a workflow run.
set -euo pipefail

artifact=${1:?artifact name required}
job=${2:?producing job name required}
timeout=${3:-1800}
runs="repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
deadline=$((SECONDS + timeout))

while ((SECONDS < deadline)); do
  if [[ -n "$(gh api "${runs}/artifacts?name=${artifact}" --jq '.artifacts[].id')" ]]; then
    echo "Artifact ${artifact} is ready after ${SECONDS}s"
    exit 0
  fi
  # A successful job may finish moments before its artifact is listed, so
  # only a job that finished any other way ends the wait early.
  conclusion=$(gh api "${runs}/attempts/${GITHUB_RUN_ATTEMPT}/jobs?per_page=100" \
    --jq ".jobs[] | select(.name == \"${job}\") | .conclusion // empty")
  if [[ -n "${conclusion}" && "${conclusion}" != "success" ]]; then
    echo "Job '${job}' finished with ${conclusion}; artifact ${artifact} will not arrive" >&2
    exit 1
  fi
  echo "Waiting for artifact ${artifact} from job '${job}' (${SECONDS}s)"
  sleep 10
done

echo "Timed out after ${timeout}s waiting for artifact ${artifact}" >&2
exit 1
