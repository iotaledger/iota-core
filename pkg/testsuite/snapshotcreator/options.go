package snapshotcreator

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	ledger1 "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// Options stores the details about snapshots created for integration tests.
type Options struct {
	// FilePath is the path to the snapshot file.
	FilePath string

	// ProtocolParameters provides the protocol parameters used for the network.
	ProtocolParameters iotago.ProtocolParameters

	// AddGenesisRootBlock defines whether a Genesis root block should be added.
	AddGenesisRootBlock bool

	// RootBlocks define the initial blocks to which new blocks can attach to.
	RootBlocks map[iotago.BlockID]iotago.CommitmentID

	// GenesisKeyManager defines the key manager used to generate keypair that can spend Genesis outputs.
	GenesisKeyManager *wallet.KeyManager

	// Accounts defines the accounts that are created in the ledger as part of the Genesis.
	Accounts []AccountDetails

	// BasicOutput defines the basic outputs that are created in the ledger as part of the Genesis.
	BasicOutputs []BasicOutputDetails

	DataBaseVersion byte
	LedgerProvider  module.Provider[*engine.Engine, ledger.Ledger]
}

func NewOptions(opts ...options.Option[Options]) *Options {
	return options.Apply(&Options{
		FilePath:        "snapshot.bin",
		DataBaseVersion: 1,
		LedgerProvider:  ledger1.NewProvider(),
	}, opts)
}

func WithLedgerProvider(ledgerProvider module.Provider[*engine.Engine, ledger.Ledger]) options.Option[Options] {
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

// WithAddGenesisRootBlock define whether a Genesis root block should be added.
func WithAddGenesisRootBlock(add bool) options.Option[Options] {
	return func(m *Options) {
		m.AddGenesisRootBlock = add
	}
}

// WithGenesisKeyManager defines the seed used to generate keypair that can spend Genesis outputs.
func WithGenesisKeyManager(keyManager *wallet.KeyManager) options.Option[Options] {
	return func(m *Options) {
		m.GenesisKeyManager = keyManager
	}
}

// AccountDetails is a struct that specifies details of accounts created in the Genesis snapshot.
// AccountID is derived from IssuerKey, therefore, this value must be unique for each account.
type AccountDetails struct {
	AccountID iotago.AccountID

	Address              iotago.Address
	Amount               iotago.BaseToken
	Mana                 iotago.Mana
	IssuerKey            iotago.BlockIssuerKey
	ExpirySlot           iotago.SlotIndex
	BlockIssuanceCredits iotago.BlockIssuanceCredits

	StakingEndEpoch iotago.EpochIndex
	FixedCost       iotago.Mana
	StakedAmount    iotago.BaseToken
}

func WithAccounts(accounts ...AccountDetails) options.Option[Options] {
	return func(m *Options) {
		m.Accounts = accounts
	}
}

// BasicOutputDetails is a struct that specifies details of a basic output created in the Genesis snapshot.
type BasicOutputDetails struct {
	Address iotago.Address
	Amount  iotago.BaseToken
	Mana    iotago.Mana
}

func WithBasicOutputs(basicOutputs ...BasicOutputDetails) options.Option[Options] {
	return func(m *Options) {
		m.BasicOutputs = basicOutputs
	}
}
