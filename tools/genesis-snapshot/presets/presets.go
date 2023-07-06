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
	snapshotcreator.WithProtocolParameters(iotago.ProtocolParameters{
		Version:     3,
		NetworkName: "default",
		Bech32HRP:   "rms",
		MinPoWScore: 10,
		RentStructure: iotago.RentStructure{
			VByteCost:    100,
			VBFactorData: 1,
			VBFactorKey:  10,
		},
		TokenSupply:           1_000_0000,
		GenesisUnixTimestamp:  time.Now().Unix(),
		SlotDurationInSeconds: 10,
		EvictionAge:           6,
		LivenessThreshold:     3,
		EpochNearingThreshold: 30,
	}),
	snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
		iotago.EmptyBlockID(): iotago.NewEmptyCommitment().MustID(),
	}),
}

var Docker = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithProtocolParameters(iotago.ProtocolParameters{
		Version:     3,
		NetworkName: "docker",
		Bech32HRP:   "rms",
		MinPoWScore: 10,
		RentStructure: iotago.RentStructure{
			VByteCost:    100,
			VBFactorData: 1,
			VBFactorKey:  10,
		},
		TokenSupply:                      1_000_0000,
		GenesisUnixTimestamp:             time.Now().Unix(),
		SlotDurationInSeconds:            10,
		SlotsPerEpochExponent:            5,
		ManaGenerationRate:               0,
		ManaGenerationRateExponent:       0,
		ManaDecayFactors:                 nil,
		ManaDecayFactorsExponent:         0,
		ManaDecayFactorEpochsSum:         0,
		ManaDecayFactorEpochsSumExponent: 0,
		StakingUnbondingPeriod:           2,
		EvictionAge:                      6,
		LivenessThreshold:                3,
		EpochNearingThreshold:            16,
	}),
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
}

var Feature = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithProtocolParameters(iotago.ProtocolParameters{
		Version:     3,
		NetworkName: "feature",
		Bech32HRP:   "rms",
		MinPoWScore: 10,
		RentStructure: iotago.RentStructure{
			VByteCost:    100,
			VBFactorData: 1,
			VBFactorKey:  10,
		},
		TokenSupply:           1_000_0000,
		GenesisUnixTimestamp:  time.Now().Unix(),
		SlotDurationInSeconds: 10,
		EvictionAge:           6,
		LivenessThreshold:     3,
		EpochNearingThreshold: 30,
	}),
}
