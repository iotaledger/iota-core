#!/bin/bash
pushd ../tools/genesis-snapshot

# determine current iota-core version tag
commit_hash=$(git rev-parse --short HEAD)

BUILD_TAGS=rocksdb
BUILD_LD_FLAGS="-s -w -X=github.com/iotaledger/iota-core/components/app.Version=${commit_hash}"

go run -tags ${BUILD_TAGS} -ldflags "${BUILD_LD_FLAGS}" main.go && mv snapshot.bin ../../

popd
