package snapshotcreator

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Options stores the details about snapshots created for integration tests.
type Options struct {
	// FilePath is the path to the snapshot file.
	FilePath string

	// ProtocolParameters provides the protocol parameters used for the network.
	ProtocolParameters iotago.ProtocolParameters

	// RootBlocks define the initial blocks to which new blocks can attach to.
	RootBlocks map[iotago.BlockID]iotago.CommitmentID

	DataBaseVersion byte
}

func NewOptions(opts ...options.Option[Options]) *Options {
	return options.Apply(&Options{
		FilePath:           "snapshot.bin",
		DataBaseVersion:    1,
		ProtocolParameters: iotago.ProtocolParameters{},
	}, opts)
}

func WithDatabaseVersion(dbVersion byte) options.Option[Options] {
	return func(m *Options) {
		m.DataBaseVersion = dbVersion
	}
}

func WithFilePath(filePath string) options.Option[Options] {
	return func(m *Options) {
		m.FilePath = filePath
	}
}

// WithProtocolParameters defines the protocol parameters used for the network.
func WithProtocolParameters(params iotago.ProtocolParameters) options.Option[Options] {
	return func(m *Options) {
		m.ProtocolParameters = params
	}
}

// WithRootBlocks define the initial blocks to which new blocks can attach to.
func WithRootBlocks(rootBlocks map[iotago.BlockID]iotago.CommitmentID) options.Option[Options] {
	return func(m *Options) {
		m.RootBlocks = rootBlocks
	}
}
