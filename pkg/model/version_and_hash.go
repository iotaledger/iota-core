package model

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const VersionAndHashSize = iotago.VersionLength + iotago.IdentifierLength

type VersionAndHash struct {
	Version iotago.Version    `serix:""`
	Hash    iotago.Identifier `serix:""`
}

func (v VersionAndHash) Bytes() ([]byte, error) {
	// iotago.Version and iotago.Identifier can't panic on .Bytes() call.
	return byteutils.ConcatBytes(lo.PanicOnErr(v.Version.Bytes()), lo.PanicOnErr(v.Hash.Bytes())), nil
}

func VersionAndHashFromBytes(bytes []byte) (VersionAndHash, int, error) {
	version, versionBytesConsumed, err := iotago.VersionFromBytes(bytes)
	if err != nil {
		return VersionAndHash{}, 0, ierrors.Wrap(err, "failed to parse version")
	}

	hash, hashBytesConsumed, err := iotago.IdentifierFromBytes(bytes[versionBytesConsumed:])
	if err != nil {
		return VersionAndHash{}, 0, ierrors.Wrap(err, "failed to parse hash")
	}

	return VersionAndHash{version, hash}, versionBytesConsumed + hashBytesConsumed, nil
}
