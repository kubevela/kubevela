#!/bin/bash

SHARD=${SHARD:-3}
BEGIN=${BEGIN:-0}
SIZE=${SIZE:-1000}
WORKER=${WORKER:-8}

run() {
  for i in $(seq $(expr $1 + $BEGIN) $WORKER $(expr $BEGIN + $SIZE - 1)); do
    kubectl delete app app-$i -n load-test-$(expr $i % $SHARD) --wait=false
  done
}

for j in $(seq 0 $WORKER); do
  run $j &
done

wait
