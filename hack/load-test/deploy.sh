#!/bin/bash

BEGIN=${BEGIN:-1}
SIZE=${SIZE:-1000}
WORKER=${WORKER:-6}
VERSION=${VERSION:-1}

SHARD=${SHARD:-3}

END=$(expr $BEGIN + $SIZE - 1)

run() {
  for i in $(seq $1 $3 $2); do
    sid=$(expr $i % $SHARD)
    v=${VERSION}
    cat ./app-templates/light.yaml | sed 's/ID/'$i'/g' | sed 's/SHARD/'$sid'/g' | sed 's/VERSION/'$v'/g' | kubectl apply -f -
    echo "worker $4: apply app $i to $sid"
  done
  echo "worker $4: done"
}

for i in $(seq 0 $(expr $SHARD - 1)); do
  kubectl create ns load-test-$i
done

for i in $(seq 1 $WORKER); do
  run $(expr $BEGIN + $i - 1) $END $WORKER $i &
done

wait