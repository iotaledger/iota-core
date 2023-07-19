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
			iotago.WithSupplyOptions(1_000_0000, 100, 1, 10),
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
		snapshotcreator.AccountDetails{
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
	),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("docker", "rms"),
			iotago.WithSupplyOptions(100_000_0000, 1, 1, 10),
			iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(6, 5, 30),
		),
	),
}

// Feature is a preset for the feature network, genesis time ~20th of July 2023.
var Feature = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithAccounts(
		snapshotcreator.AccountDetails{
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xa73e11cd633fe55e04ac9f820eccfb2fc7c93213329e04e2ae71e6c821226a60"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xa73e11cd633fe55e04ac9f820eccfb2fc7c93213329e04e2ae71e6c821226a60"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0xa73e11cd633fe55e04ac9f820eccfb2fc7c93213329e04e2ae71e6c821226a60")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xeeed3d9c0e1bc8968b92c0aa25661fb01ab52d2204eba2e236249ea236e5a91d"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xeeed3d9c0e1bc8968b92c0aa25661fb01ab52d2204eba2e236249ea236e5a91d"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0xeeed3d9c0e1bc8968b92c0aa25661fb01ab52d2204eba2e236249ea236e5a91d")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
		snapshotcreator.AccountDetails{
			AccountID:       blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xa3bc64bc420e3b163c7c17176c3153febcd493a57e558f7b8c7fc300e2b1b2e8"))),
			Address:         iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xa3bc64bc420e3b163c7c17176c3153febcd493a57e558f7b8c7fc300e2b1b2e8"))),
			Amount:          testsuite.MinValidatorAccountDeposit,
			IssuerKey:       lo.PanicOnErr(hexutil.DecodeHex("0xa3bc64bc420e3b163c7c17176c3153febcd493a57e558f7b8c7fc300e2b1b2e8")),
			StakingEpochEnd: math.MaxUint64,
			FixedCost:       1,
			StakedAmount:    testsuite.MinValidatorAccountDeposit,
		},
	),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("feature", "rms"),
			iotago.WithSupplyOptions(100_000_0000, 1, 1, 10),
			iotago.WithTimeProviderOptions(1689844520, 10, 13),
			iotago.WithLivenessOptions(6, 5, 30),
		),
	),
}
