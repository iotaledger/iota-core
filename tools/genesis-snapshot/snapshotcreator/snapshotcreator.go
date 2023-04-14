package snapshotcreator

import (
	"os"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker/inmemorybooker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

// CreateSnapshot creates a new snapshot. Genesis is defined by genesisTokenAmount and seedBytes, it
// is pledged to the node that is derived from the same seed. The amount to pledge to each node is defined by
// nodesToPledge map (publicKey->amount), the funds of each pledge is sent to the same publicKey.
// | Pledge      | Funds       |
// | ----------- | ----------- |
// | empty       | genesisSeed |
// | node1       | node1       |
// | node2       | node2       |.

func CreateSnapshot(opts ...options.Option[Options]) error {
	opt := NewOptions(opts...)

	workers := workerpool.NewGroup("CreateSnapshot")
	defer workers.Shutdown()
	s := storage.New(lo.PanicOnErr(os.MkdirTemp(os.TempDir(), "*")), opt.DataBaseVersion)
	defer s.Shutdown()

	if err := s.Commitments.Store(iotago.NewEmptyCommitment()); err != nil {
		return errors.Wrap(err, "failed to store empty commitment")
	}
	if err := s.Settings.SetProtocolParameters(opt.ProtocolParameters); err != nil {
		return errors.Wrap(err, "failed to set the genesis time")
	}
	if err := s.Settings.SetChainID(lo.PanicOnErr(s.Commitments.Load(0)).MustID()); err != nil {
		return errors.Wrap(err, "failed to set chainID")
	}

	engineInstance := engine.New(workers.CreateGroup("Engine"),
		s,
		blockfilter.NewProvider(),
		inmemoryblockdag.NewProvider(),
		inmemorybooker.NewProvider(),
	)
	defer engineInstance.Shutdown()

	for blockID, commitmentID := range opt.RootBlocks {
		engineInstance.EvictionState.AddRootBlock(blockID, commitmentID)
	}

	if err := engineInstance.WriteSnapshot(opt.FilePath); err != nil {
		return err
	}
	return nil
}
