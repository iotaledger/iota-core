package presets

import (
	"math"
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
)

var Base = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
	snapshotcreator.WithFilePath("snapshot.bin"),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("default", "rms"),
			iotago.WithSupplyOptions(10_000_000_000, 100, 1, 10),
			iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(6, 5, 30),
		),
	),
	snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
		iotago.EmptyBlockID(): iotago.NewEmptyCommitment(3).MustID(),
	}),
}

var Docker = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithAccounts(
		snapshotcreator.AccountDetails{ // master
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{ // master2
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{ // faucet
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{ // nomana
			AccountID: blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xa54fafa44a88e4a6a37796526ea884f613a24d84337871226eb6360f022d8b39"))),
			Address:   iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xa54fafa44a88e4a6a37796526ea884f613a24d84337871226eb6360f022d8b39"))),
			Amount:    testsuite.MinIssuerAccountDeposit,
			IssuerKey: lo.PanicOnErr(hexutil.DecodeHex("0xa54fafa44a88e4a6a37796526ea884f613a24d84337871226eb6360f022d8b39")),
		},
		snapshotcreator.AccountDetails{ // nomana2
			AccountID: blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xcb5ea14175ce649149ee41217c44aa70c3205b9939968449eae408727a71f91b"))),
			Address:   iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xcb5ea14175ce649149ee41217c44aa70c3205b9939968449eae408727a71f91b"))),
			Amount:    testsuite.MinIssuerAccountDeposit,
			IssuerKey: lo.PanicOnErr(hexutil.DecodeHex("0xcb5ea14175ce649149ee41217c44aa70c3205b9939968449eae408727a71f91b")),
		},
	),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("docker", "rms"),
			iotago.WithSupplyOptions(10_000_000_000, 1, 1, 10),
			iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(6, 5, 30),
		),
	),
}

var Feature = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("feature", "rms"),
			iotago.WithSupplyOptions(10_000_000_000, 100, 1, 10),
			iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(6, 5, 30),
		),
	),
}
