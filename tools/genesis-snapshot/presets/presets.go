package presets

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
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
		GenesisUnixTimestamp:  uint32(time.Now().Unix()),
		SlotDurationInSeconds: 10,
		EpochDurationInSlots:  8192,
		MaxCommittableAge:     10,
		ProtocolVersions: []iotago.ProtocolVersion{
			{Version: 3, StartEpoch: 0},
		},
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
		TokenSupply:           1_000_0000,
		GenesisUnixTimestamp:  uint32(time.Now().Unix()),
		SlotDurationInSeconds: 10,
		EpochDurationInSlots:  8192,
		MaxCommittableAge:     10,
		ProtocolVersions: []iotago.ProtocolVersion{
			{Version: 3, StartEpoch: 0},
		},
	}),
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
		GenesisUnixTimestamp:  uint32(time.Now().Unix()),
		SlotDurationInSeconds: 10,
		EpochDurationInSlots:  8192,
		MaxCommittableAge:     10,
		ProtocolVersions: []iotago.ProtocolVersion{
			{Version: 3, StartEpoch: 0},
		},
	}),
}
