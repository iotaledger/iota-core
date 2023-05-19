package snapshotcreator

import (
	"os"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker/inmemorybooker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock/blocktime"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
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

	api := iotago.LatestAPI(&opt.ProtocolParameters)

	errorHandler := func(err error) {
		panic(err)
	}

	workers := workerpool.NewGroup("CreateSnapshot")
	defer workers.Shutdown()
	s := storage.New(lo.PanicOnErr(os.MkdirTemp(os.TempDir(), "*")), opt.DataBaseVersion, errorHandler)
	defer s.Shutdown()

	if err := s.Commitments().Store(model.NewEmptyCommitment(api)); err != nil {
		return errors.Wrap(err, "failed to store empty commitment")
	}
	if err := s.Settings().SetProtocolParameters(opt.ProtocolParameters); err != nil {
		return errors.Wrap(err, "failed to set the genesis time")
	}

	engineInstance := engine.New(workers.CreateGroup("Engine"),
		errorHandler,
		s,
		blockfilter.NewProvider(),
		inmemoryblockdag.NewProvider(),
		inmemorybooker.NewProvider(),
		blocktime.NewProvider(),
		poa.NewProvider(map[iotago.AccountID]int64{}),
		thresholdblockgadget.NewProvider(),
		totalweightslotgadget.NewProvider(),
		slotnotarization.NewProvider(),
		opt.LedgerProvider(),
	)
	defer engineInstance.Shutdown()

	for blockID, commitmentID := range opt.RootBlocks {
		engineInstance.EvictionState.AddRootBlock(blockID, commitmentID)
	}

	createGenesisOutput(100, []byte{}, engineInstance)

	return engineInstance.WriteSnapshot(opt.FilePath)
}

func createGenesisOutput(genesisTokenAmount uint64, genesisSeed []byte, engineInstance *engine.Engine) error {
	if genesisTokenAmount > 0 {
		output, outputMetadata, err := createOutput(engineInstance.Ledger, seed.NewSeed(m.GenesisSeed).KeyPair(0).PublicKey, m.GenesisTokenAmount, identity.ID{}, 0)
		if err != nil {
			return err
		}

		if err = engineInstance.Ledger.AddUnspentOutput(mempool.NewOutputWithMetadata(0, output.ID(), output, outputMetadata.ConsensusManaPledgeID(), outputMetadata.AccessManaPledgeID())); err != nil {
			return err
		}
	}
	return nil
}

func createOutput(ledgerVM vm.VM, publicKey ed25519.PublicKey, tokenAmount uint64, pledgeID identity.ID, includedInSlot slot.Index) (output utxo.Output, outputMetadata *mempool.OutputMetadata, err error) {
	switch ledgerVM.(type) {
	case *mockedvm.MockedVM:
		output = mempooltests.
		(utxo.EmptyTransactionID, outputCounter, tokenAmount)

	case *devnetvm.VM:
		output = devnetvm.NewSigLockedColoredOutput(devnetvm.NewColoredBalances(map[devnetvm.Color]uint64{
			devnetvm.ColorIOTA: tokenAmount,
		}), devnetvm.NewED25519Address(publicKey))
		output.SetID(utxo.NewOutputID(utxo.EmptyTransactionID, outputCounter))

	default:
		return nil, nil, errors.Errorf("cannot create snapshot output for VM of type '%v'", ledgerVM)
	}

	outputCounter++

	outputMetadata = mempool.NewOutputMetadata(output.ID())
	outputMetadata.SetConfirmationState(confirmation.Confirmed)
	outputMetadata.SetAccessManaPledgeID(pledgeID)
	outputMetadata.SetConsensusManaPledgeID(pledgeID)
	outputMetadata.SetInclusionSlot(includedInSlot)

	return output, outputMetadata, nil
}
