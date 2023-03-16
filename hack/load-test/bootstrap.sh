#!/bin/bash

SHARD=${SHARD:-3}

for i in $(seq 0 $(expr $SHARD - 1)); do
  kubectl create ns load-test-$i
done

for i in $(seq 0 $(expr $SHARD - 1)); do
  echo "
    apiVersion: v1
    kind: Namespace
    metadata:
      name: load-test-$i
  " > /tmp/ns-$i.yaml
  kubectl get clustergateways | grep cluster- | awk '{print $1}' | xargs -n1 -P8 vela kube apply -f /tmp/ns-$i.yaml --cluster
done