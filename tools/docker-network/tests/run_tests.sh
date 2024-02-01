#!/bin/bash

BUILD_TAGS="rocksdb,dockertests"
TIMEOUT=120m

# Exit script on non-zero command exit status
set -e

# Change directory to the parent directory of this script
pushd ../

# Build the docker image with buildkit
echo "Build iota-core docker image"
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1
export DOCKER_BUILD_CONTEXT="../../"
export DOCKERFILE_PATH="./Dockerfile.dev"

# Allow docker compose to build and cache an image
docker compose build --build-arg DOCKER_BUILD_CONTEXT=${DOCKER_BUILD_CONTEXT} --build-arg DOCKERFILE_PATH=${DOCKERFILE_PATH}

# Pull missing images
docker compose pull inx-indexer inx-mqtt inx-blockissuer inx-faucet inx-validator-1

# Change directory back to the original directory
popd

# If no arguments were passed, run all tests
if [ $# -eq 0 ]; then
    echo "Running all tests..."
    go test ./... -tags ${BUILD_TAGS} -v -timeout=${TIMEOUT}
else
    # Concatenate all test names with a pipe
    tests=$(printf "|%s" "$@")
    tests=${tests:1}

    echo "Running tests: $tests..."

    # Run the specific tests
    go test -run=$tests -tags ${BUILD_TAGS} -v -timeout=${TIMEOUT}
fi