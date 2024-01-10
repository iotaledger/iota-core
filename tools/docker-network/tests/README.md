# Docker Tests

These tests and the `DockerTestFramework` are using the Docker network and simulate a real network environment with multiple nodes.
They are therefore fully-fledged integration tests that test the entire node and inx-* connections.

The tests are by default excluded from compilation and tests with `//go:build dockertests`. To run the tests, the `dockertests` build tag must be added when compiling. 

## Prerequisites
Before running the tests make sure to build the latest image and pull the latest images from Docker Hub.

```bash
cd tools/docker-network
docker compose build
docker compose pull inx-indexer inx-mqtt inx-blockissuer inx-faucet inx-validator-1
```

## Running the tests
To run the tests, simply execute the following command:

```bash
# run all tests
go test ./... -tags rocksdb,dockertests -v -timeout=60m

# or to run a specific test 
go test -run=Test_Delegation -tags rocksdb,dockertests -v -timeout=60m
```
