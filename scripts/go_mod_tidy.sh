#!/bin/bash
pushd ./..

go mod tidy

pushd tools/gendoc
go mod tidy
popd

popd