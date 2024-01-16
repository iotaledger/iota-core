#!/bin/bash

DEFAULT_TEST_NAMES='SmallerCommittee ReuseDueToNoFinalization NoCandidacyPayload Staking Delegation'
TEST_NAMES=${1:-$DEFAULT_TEST_NAMES}

export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1
echo "Build iota-core"

# Setup necessary environment variables.
export DOCKER_BUILD_CONTEXT="../../"
export DOCKERFILE_PATH="./Dockerfile.dev"

if [[ "$WITH_GO_WORK" -eq 1 ]]; then
  export DOCKER_BUILD_CONTEXT="../../../"
  export DOCKERFILE_PATH="./iota-core/Dockerfile.dev"
fi

# Allow docker compose to build and cache an image
echo $DOCKER_BUILD_CONTEXT $DOCKERFILE_PATH
docker compose build --build-arg WITH_GO_WORK=${WITH_GO_WORK:-0} --build-arg DOCKER_BUILD_CONTEXT=${DOCKER_BUILD_CONTEXT} --build-arg DOCKERFILE_PATH=${DOCKERFILE_PATH}

docker compose pull inx-indexer inx-mqtt inx-blockissuer inx-faucet inx-validator-1

# check exit code of builder
if [ $? -ne 0 ]; then
  echo "Building failed. Please fix and try again!"
  exit 1
fi

# run tests
pushd tests

for name in $TEST_NAMES; do
  go test -run="Test_"$name -tags rocksdb,dockertests -v -timeout=30m
done

popd