#!/usr/bin/env bash
# Stages a deterministic 2-chain on one column: [blue,blue,blue,red,red,red,red,blue]
# bottom-up. The reds erase first (chain 1), the blues compact into four (chain 2).
set -eu
NODE=${1:-kbkb-worker4}
BASE=$(date +%s%N)
i=0
for color in blue blue blue red red red red blue; do
  i=$((i + 1))
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: chain-$i
  namespace: default
  annotations:
    kbkb.k8s.omakenoyouna.net/color: "$color"
    kbkb.k8s.omakenoyouna.net/drop-order: "$((BASE + i))"
spec:
  nodeName: $NODE
  terminationGracePeriodSeconds: 1
  containers:
  - name: puyo
    image: registry.k8s.io/pause:3.10
EOF
done
