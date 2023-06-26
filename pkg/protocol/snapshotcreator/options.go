package snapshotcreator

import (
	"crypto/ed25519"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	ledger1 "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/ledger"
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

	// GenesisSeed defines the seed used to generate keypair that can spend Genesis outputs.
	GenesisSeed []byte

	// Accounts defines the accounts that are created in the ledger as part of the Genesis.
	Accounts []AccountDetails

	DataBaseVersion byte
	LedgerProvider  func() module.Provider[*engine.Engine, ledger.Ledger]
}

func NewOptions(opts ...options.Option[Options]) *Options {
	return options.Apply(&Options{
		FilePath:           "snapshot.bin",
		DataBaseVersion:    1,
		ProtocolParameters: iotago.ProtocolParameters{},
		LedgerProvider:     ledger1.NewProvider,
	}, opts)
}

func WithLedgerProvider(ledgerProvider func() module.Provider[*engine.Engine, ledger.Ledger]) options.Option[Options] {
	return func(m *Options) {
		m.LedgerProvider = ledgerProvider
	}
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

// WithGenesisSeed defines the seed used to generate keypair that can spend Genesis outputs.
func WithGenesisSeed(genesisSeed []byte) options.Option[Options] {
	return func(m *Options) {
		m.GenesisSeed = genesisSeed
	}
}

type AccountDetails struct {
	Address   iotago.Address
	Amount    iotago.BaseToken
	IssuerKey ed25519.PublicKey
}

func WithAccounts(accounts ...AccountDetails) options.Option[Options] {
	return func(m *Options) {
		m.Accounts = accounts
	}
}
