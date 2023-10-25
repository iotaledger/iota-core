#!/bin/bash
set -e

# Create a function to join an array of strings by a given character
function join { local IFS="$1"; shift; echo "$*"; }

# All parameters can be optional now, just make sure we don't have too many
if [[ $# -gt 4 ]] ; then
  echo 'Call with ./run [replicas=1|2|3|...] [monitoring=0|1] [feature=0|1]'
  exit 0
fi

REPLICAS=${1:-1}
MONITORING=${2:-0}
FEATURE=${3:-0}

DOCKER_COMPOSE_FILE=docker-compose.yml
if [ $FEATURE -ne 0 ]; then
  DOCKER_COMPOSE_FILE=docker-compose-feature.yml
fi

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
docker compose -f $DOCKER_COMPOSE_FILE build --build-arg WITH_GO_WORK=${WITH_GO_WORK:-0} --build-arg DOCKER_BUILD_CONTEXT=${DOCKER_BUILD_CONTEXT} --build-arg DOCKERFILE_PATH=${DOCKERFILE_PATH}

docker compose pull inx-indexer # inx-blockissuer inx-faucet

# check exit code of builder
if [ $? -ne 0 ]; then
  echo "Building failed. Please fix and try again!"
  exit 1
fi

# create snapshot file
echo "Create snapshot"
if [ $FEATURE -ne 0 ]; then
  pushd ../genesis-snapshot
  go run -tags=rocksdb . --config feature
else
  pushd ../genesis-snapshot
  go run -tags=rocksdb . --config docker --seed 7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih
fi
popd
mv ../genesis-snapshot/*.snapshot .
chmod o+r *.snapshot

echo "Run iota-core network with ${DOCKER_COMPOSE_FILE}"
# IOTA_CORE_PEER_REPLICAS is used in docker-compose.yml to determine how many replicas to create
export IOTA_CORE_PEER_REPLICAS=$REPLICAS
# Profiles is created to set which docker profiles to run
# https://docs.docker.com/compose/profiles/
PROFILES=()
if [ $MONITORING -ne 0 ]; then
  PROFILES+=("monitoring")
fi

export COMPOSE_PROFILES=$(join , ${PROFILES[@]})
docker compose -f $DOCKER_COMPOSE_FILE up

echo "Clean up docker resources"
docker compose down -v
rm *.snapshot
