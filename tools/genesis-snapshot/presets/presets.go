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
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("docker", "rms"),
			iotago.WithSupplyOptions(100_000_0000, 1, 1, 10),
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
			iotago.WithSupplyOptions(1_000_0000, 100, 1, 10),
			iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(6, 5, 30),
		),
	),
}
