#!/bin/bash
#
# Builds both the node dashboard and dagvisualizer

echo "::: Building /plugins/dashboard/frontend :::"
rm -rf /frontend/build

docker run -it --rm \
    -u $(id -u ${USER}):$(id -g ${USER}) \
    --volume="/etc/group:/etc/group:ro" \
    --volume="/etc/passwd:/etc/passwd:ro" \
    --volume="/etc/shadow:/etc/shadow:ro" \
    -v $(pwd):/tmp/mnt \
    -e YARN_CACHE_FOLDER=/tmp/ \
    -e HOME=/tmp/ \
    -w /tmp/mnt/frontend node:14.0 bash -c "yarn install && yarn build"
