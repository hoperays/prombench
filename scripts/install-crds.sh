#!/usr/bin/env bash
set -euo pipefail

crds_dir=$(dirname $0)/../crds/

for crd_file in $(find $crds_dir -type f -name '*.yaml'); do
  kubectl create -f $crd_file || kubectl replace -f $crd_file || true
done
