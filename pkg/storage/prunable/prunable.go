package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	blocksPrefix byte = iota
	rootBlocksPrefix
	attestationsPrefix
	ledgerStateDiffsPrefix
)

type Prunable struct {
	manager          *Manager
	Blocks           *Blocks
	RootBlocks       *RootBlocks
	Attestations     func(index iotago.SlotIndex) kvstore.KVStore
	LedgerStateDiffs func(index iotago.SlotIndex) kvstore.KVStore
}

func New(dir *utils.Directory, version database.Version, dbEngine hivedb.Engine, opts ...options.Option[Manager]) *Prunable {
	manager := NewManager(dir.Path(), version, dbEngine, opts...)

	return &Prunable{
		manager:          manager,
		Blocks:           NewBlocks(manager, blocksPrefix),
		RootBlocks:       NewRootBlocks(manager, rootBlocksPrefix),
		Attestations:     lo.Bind([]byte{attestationsPrefix}, manager.Get),
		LedgerStateDiffs: lo.Bind([]byte{ledgerStateDiffsPrefix}, manager.Get),
	}
}

func (p *Prunable) Shutdown() {
	p.manager.Shutdown()
}
