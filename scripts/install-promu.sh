#!/usr/bin/env bash
set -euo pipefail

GOPATH=$(go env GOPATH)
GOHOSTOS=$(go env GOHOSTOS)
GOHOSTARCH=$(go env GOHOSTARCH)

PROMU_VERSION=0.15.0
GO_BUILD_PLATFORM=${GOHOSTOS}-${GOHOSTARCH}
PROMU_URL=https://github.com/prometheus/promu/releases/download/v${PROMU_VERSION}/promu-${PROMU_VERSION}.${GO_BUILD_PLATFORM}.tar.gz

PROMU_TMP=$(mktemp -d)
curl -s -L ${PROMU_URL} | tar -xvzf - -C ${PROMU_TMP}
mkdir -p ${GOPATH}/bin
cp ${PROMU_TMP}/promu-${PROMU_VERSION}.${GO_BUILD_PLATFORM}/promu ${GOPATH}/bin/promu
rm -r ${PROMU_TMP}
