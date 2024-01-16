# IOTA-Core - The IOTA 2.0 node

IOTA-Core is the node software for the upcoming IOTA 2.0 protocol.

---
![GitHub Release (latest by date)](https://img.shields.io/github/v/release/iotaledger/iota-core)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/iotaledger/iota-core)
![GitHub License](https://img.shields.io/github/license/iotaledger/iota-core)
---
[![build_docker](https://github.com/iotaledger/iota-core/actions/workflows/build_docker.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/build_docker.yml)
[![build_tools](https://github.com/iotaledger/iota-core/actions/workflows/build_tools.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/build_tools.yml)
[![docker-network-health](https://github.com/iotaledger/iota-core/actions/workflows/docker-network-health.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/docker-network-health.yml)
[![docker-network-tests-nightly](https://github.com/iotaledger/iota-core/actions/workflows/docker-network-tests-nightly.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/docker-network-tests-nightly.yml)
[![golangci-lint](https://github.com/iotaledger/iota-core/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/golangci-lint.yml)
[![release](https://github.com/iotaledger/iota-core/actions/workflows/release.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/release.yml)
[![unit-test](https://github.com/iotaledger/iota-core/actions/workflows/unit-test.yml/badge.svg)](https://github.com/iotaledger/iota-core/actions/workflows/unit-test.yml)
---

In this repository you will find the following branches:

- `production`: this branch contains the latest released code targeted for the [IOTA mainnet](https://iota.org)
- `staging`: this branch contains the latest released code targeted for the [shimmer network](https://shimmer.network)
- `develop`: default branch where all development will get merged to. This represents the next iteration of the node.

## Notes

- **Please open a [new issue](https://github.com/iotaledger/iota-core/issues/new) if you detect an error or crash (or submit a PR if you have already fixed it).**

## Configuration

An overview over all configuration parameters can be found [here.](documentation/configuration.md)

## Setup
We recommend not using this repo directly but using our pre-built [Docker images](https://hub.docker.com/r/iotaledger/iota-core).
