#!/bin/bash

COMMIT=$1
MODULES="ads app autopeering constraints core crypto ds kvstore lo logger objectstorage runtime serializer/v2 stringify"

if [ -z "$COMMIT" ]
then
    echo "ERROR: no commit hash given!"
    exit 1
fi

for i in $MODULES
do
	go get -u github.com/iotaledger/hive.go/$i@$COMMIT
done

pushd $(dirname $0)
./go_mod_tidy.sh
popd
