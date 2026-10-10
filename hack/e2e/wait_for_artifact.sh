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
  # "pending" while the job runs; empty when this attempt does not include it.
  state=$(gh api "${runs}/attempts/${GITHUB_RUN_ATTEMPT}/jobs?per_page=100" \
    --jq ".jobs[] | select(.name == \"${job}\") | .conclusion // \"pending\"")
  listed=$(gh api "${runs}/artifacts?name=${artifact}" --jq '.artifacts[].id')
  case "${state}" in
    success)
      # The job finishes only after its upload step has finalized the
      # artifact; it can take a moment to appear in the list.
      if [[ -n "${listed}" ]]; then
        echo "Artifact ${artifact} is ready after ${SECONDS}s"
        exit 0
      fi
      ;;
    "")
      # A re-run of failed jobs only: the image comes from an earlier attempt,
      # whose upload finished with that attempt.
      if [[ -n "${listed}" ]]; then
        echo "Artifact ${artifact} from an earlier attempt is ready"
        exit 0
      fi
      ;;
    pending) ;;
    *)
      echo "Job '${job}' finished with ${state}; artifact ${artifact} will not arrive" >&2
      exit 1
      ;;
  esac
  echo "Waiting for artifact ${artifact} from job '${job}' (${SECONDS}s, job ${state:-not in this attempt})"
  sleep 10
done

echo "Timed out after ${timeout}s waiting for artifact ${artifact}" >&2
exit 1
