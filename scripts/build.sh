#!/bin/bash
#
# Builds iota-core with the latest commit hash (short)
# E.g.: ./iota-core -v --> iota-core 75316fe
pushd ./..

commit_hash=$(git rev-parse --short HEAD)

BUILD_TAGS=rocksdb
BUILD_LD_FLAGS="-s -w -X=github.com/iotaledger/iota-core/components/app.Version=${commit_hash}"

go build -tags ${BUILD_TAGS} -ldflags "${BUILD_LD_FLAGS}"

popd
