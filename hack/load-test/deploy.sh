#!/bin/bash

BEGIN=${BEGIN:-1}
SIZE=${SIZE:-1000}
WORKER=${WORKER:-6}
VERSION=${VERSION:-1}
CLUSTER=${CLUSTER:-4}
QPS=${QPS:-1}

SHARD=${SHARD:-3}

TEMPLATE=${TEMPLATE:-"light"}

END=$(expr $BEGIN + $SIZE - 1)

waitTime=$(expr 1000 / $QPS)e-3

run() {
  for i in $(seq $1 $3 $2); do
    sid=$(expr $i % $SHARD)
    v=${VERSION}
    c=$(expr $i % $CLUSTER)
    cat ./app-templates/$TEMPLATE.yaml | \
      sed 's/ID/'$i'/g' | \
      sed 's/SHARD/'$sid'/g' | \
      sed 's/VERSION/'$v'/g' | \
      sed 's/CLUSTER/'$c'/g' | \
      kubectl apply -f -
    echo "worker $4: apply app $i to $sid"
    sleep $waitTime
  done
  echo "worker $4: done"
}

for i in $(seq 1 $WORKER); do
  run $(expr $BEGIN + $i - 1) $END $WORKER $i &
done

wait
