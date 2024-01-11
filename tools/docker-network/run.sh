#!/bin/bash

# Create a function to join an array of strings by a given character
function join { local IFS="$1"; shift; echo "$*"; }

# All parameters can be optional now, just make sure we don't have too many
if [[ $# -gt 2 ]] ; then
  echo 'Call with ./run.sh [monitoring=0|1]'
  exit 0
fi

MONITORING=${1:-0}

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

# create snapshot file
echo "Create snapshot"

# Run Go command in Docker container
docker run --rm \
  --user $(id -u) \
  -v "$(realpath $(pwd)/../../):/workspace" \
  -v "${HOME}/.cache/go-build:/go-cache" \
  -v "${HOME}/go/pkg/mod:/go-mod-cache" \
  -e GOCACHE="/go-cache" \
  -e GOMODCACHE="/go-mod-cache" \
  -w "/workspace/tools/genesis-snapshot" \
  golang:1.21-bookworm go run -tags=rocksdb . --config docker --seed 7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih

# Move and set permissions for the .snapshot file
if ! mv -f ../genesis-snapshot/*.snapshot .; then
  echo "Failed to create or move snapshot file"
  exit 1
fi

chmod o+r *.snapshot

echo "Run iota-core network"
# IOTA_CORE_PEER_REPLICAS is used in docker-compose.yml to determine how many replicas to create
export IOTA_CORE_PEER_REPLICAS=$REPLICAS
# Profiles is created to set which docker profiles to run
# https://docs.docker.com/compose/profiles/
PROFILES=()
if [ $MONITORING -ne 0 ]; then
  PROFILES+=("monitoring")
fi

export COMPOSE_PROFILES=$(join , ${PROFILES[@]})
docker compose up

echo "Clean up docker resources"
docker compose down -v
rm *.snapshot
