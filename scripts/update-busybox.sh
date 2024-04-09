#!/usr/bin/env bash
set -euo pipefail

# This script is called via .github/workflows/container-version.yaml
# No need to manually run this (unless you want to force an update NOW)

# Get the tags from the registry, so we can get the base manifest_digest ID
echo "Doing CURL request 1/2: getting tags."
CURL_TAGS=$(curl --fail --silent --show-error -H "Content-type: application/json" -H "Accept: application/json" https://quay.io/api/v1/repository/prometheus/busybox/tag/ 2>&1)
if [ $? -ne 0 ]; then
  echo "Error: ""$CURL_TAGS"
  exit 1
fi

MANIFEST_DIGEST=$(echo "${CURL_TAGS}" | jq -r '.tags[]' | jq -r -n 'first(inputs | select (.name=="latest")) | .manifest_digest ')

# With this manifest_digest, we can now fetch the actual manifest, which contains the digest per platform
echo "Doing CURL request 2/2: getting manifest."
RESULT_CURL=$(curl --fail --silent --show-error -H "Content-type: application/json" -H "Accept: application/json" https://quay.io/api/v1/repository/prometheus/busybox/manifest/${MANIFEST_DIGEST} 2>&1)
if [ $? -ne 0 ]; then
  echo "Error: ""$RESULT_CURL"
  exit 1
fi

# Output this as file
echo "Creating result and writing to .busybox-versions."
RESULT=$(echo "${RESULT_CURL}" | jq -r '.manifest_data | fromjson | .manifests[] | .platform.architecture +"="+ .digest' | sed 's/sha256://g' | grep -E 'amd64|arm64')
echo "# Auto generated by scripts/update-busybox.sh. DO NOT EDIT" >./.busybox-versions
echo "${RESULT}" >>./.busybox-versions
