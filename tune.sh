#!/usr/bin/env bash
set -euo pipefail

# kubectl patch prom -n prombench bench --type='json' -p='[{"op": "replace", "path": "/spec/shards", "value": 2}]'

# sort_desc(irate(container_cpu_usage_seconds_total{namespace="prombench"}[5m]) / ignoring(cpu) (container_spec_cpu_quota / container_spec_cpu_period))
# sort_desc(container_memory_working_set_bytes{namespace="prombench"} / (container_spec_memory_limit_bytes > 0))

for i in $(seq 0 4); do
  TAIL=dup-$i
  cat ./04-kubelet.yaml | sed "s/__TAIL__/$TAIL/g" | kubectl create -f -
done
