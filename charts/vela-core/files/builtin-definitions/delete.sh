#!/bin/sh
# Runs before uninstall. The release no longer owns the builtin definitions, so
# remove them here, as uninstalling did when they were part of the release.
#
# Env: CONTROLLER_NAMESPACE, RELEASE_NAME
set -eu

# Only during a Helm uninstall. Applied from `helm template` output, where hook
# annotations mean nothing, this Job would otherwise run at install time and
# remove the definitions the apply Job installs.
uninstalling=$(kubectl -n "$CONTROLLER_NAMESPACE" get secrets,configmaps \
  -l "owner=helm,name=$RELEASE_NAME,status=uninstalling" -o name)
if [ -z "$uninstalling" ]; then
  echo "No Helm uninstall of $RELEASE_NAME in progress, leaving the builtin definitions in place"
  exit 0
fi

set -- /definitions/*.yaml
if [ ! -e "$1" ]; then
  exit 0
fi

kubectl delete -f /definitions --ignore-not-found --wait=false
