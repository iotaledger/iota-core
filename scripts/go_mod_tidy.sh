#!/bin/bash
pushd ./..

go mod tidy

pushd tools/gendoc
go mod tidy
popd

pushd tools/genesis-snapshot
go mod tidy
popd

pushd tools/evil-spammer
go mod tidy
popd

popd