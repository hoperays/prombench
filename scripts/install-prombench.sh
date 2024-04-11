#!/usr/bin/env bash
set -euo pipefail

namespace=${1:-prombench}

k8s_dir=$(dirname $0)/../k8s/
tmp_dir=$(mktemp -d)

for k8s_file in $(find $k8s_dir -type f -name '*.yaml'); do
  file=$(echo $k8s_file | awk -F'/' '{print $NF}')
  sed -e "s/: prombench/: $namespace/g" \
    -e "s/- prombench/- $namespace/g" \
    -e "s/=prombench/=$namespace/g" $k8s_file >$tmp_dir/$file
  kubectl create -f $tmp_dir/$file || kubectl replace -f $tmp_dir/$file || true
done

rm -rf $tmp_dir
