package snapshotcreator

import (
	"os"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker/inmemorybooker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock/blocktime"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
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
		slotnotarization.NewProvider(slotnotarization.DefaultMinSlotCommittableAge),
		slotattestation.NewProvider(slotattestation.DefaultAttestationCommitmentOffset),
		opt.LedgerProvider(),
		tipmanagerv1.NewProvider(),
	)
	defer engineInstance.Shutdown()

	for blockID, commitmentID := range opt.RootBlocks {
		engineInstance.EvictionState.AddRootBlock(blockID, commitmentID)
	}

	if err := createGenesisOutput(opt.ProtocolParameters.TokenSupply, opt.GenesisSeed, engineInstance); err != nil {
		return errors.Wrap(err, "failed to create genesis outputs")
	}

	return engineInstance.WriteSnapshot(opt.FilePath)
}

func createGenesisOutput(genesisTokenAmount uint64, genesisSeed []byte, engineInstance *engine.Engine) error {
	if genesisTokenAmount > 0 {
		genesisWallet := mock.NewHDWallet("genesis", genesisSeed, 0)
		output := createOutput(genesisWallet.Address(), genesisTokenAmount)

		if err := engineInstance.Ledger.AddUnspentOutput(utxoledger.CreateOutput(engineInstance.API(), iotago.OutputIDFromTransactionIDAndIndex(iotago.TransactionID{}, 0), iotago.EmptyBlockID(), 0, engineInstance.API().SlotTimeProvider().GenesisTime(), output)); err != nil {
			return err
		}
	}

	return nil
}

func createOutput(address iotago.Address, tokenAmount uint64) (output iotago.Output) {
	//switch ledgerVM.(type) {
	//case *mockedvm.MockedVM:
	//case *devnetvm.VM:
	//default:
	//	return nil, nil, errors.Errorf("cannot create snapshot output for VM of type '%v'", ledgerVM)
	//}

	return &iotago.BasicOutput{
		Amount: tokenAmount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: address},
		},
	}
}
