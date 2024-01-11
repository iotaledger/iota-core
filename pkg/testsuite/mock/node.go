package mock

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// idAliases contains a list of aliases registered for a set of IDs.
var idAliases = make(map[peer.ID]string)

// RegisterIDAlias registers an alias that will modify the String() output of the ID to show a human
// readable string instead of the base58 encoded version of itself.
func RegisterIDAlias(id peer.ID, alias string) {
	idAliases[id] = alias
}

// UnregisterIDAliases removes all aliases registered through the RegisterIDAlias function.
func UnregisterIDAliases() {
	idAliases = make(map[peer.ID]string)
}

type InvalidSignedTransactionEvent struct {
	Metadata mempool.SignedTransactionMetadata
	Error    error
}

type Node struct {
	Testing *testing.T
	logger  log.Logger

	Name       string
	Validator  *BlockIssuer
	KeyManager *wallet.KeyManager

	ctx       context.Context
	ctxCancel context.CancelFunc

	PeerID                  peer.ID
	protocolParametersHash  iotago.Identifier
	highestSupportedVersion iotago.Version

	Partition string
	Endpoint  *Endpoint
	Workers   *workerpool.Group

	Protocol *protocol.Protocol

	forkDetectedCount             atomic.Uint32
	candidateEngineActivatedCount atomic.Uint32
	mainEngineSwitchedCount       atomic.Uint32

	enableEngineLogging bool

	mutex                    syncutils.RWMutex
	attachedBlocks           []*blocks.Block
	currentSlot              iotago.SlotIndex
	filteredBlockEvents      []*postsolidfilter.BlockFilteredEvent
	invalidTransactionEvents map[iotago.SignedTransactionID]InvalidSignedTransactionEvent
}

func NewNode(t *testing.T, parentLogger log.Logger, net *Network, partition string, name string, validator bool) *Node {
	keyManager := lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath))
	priv, pub := keyManager.KeyPair()

	accountID := iotago.AccountID(blake2b.Sum256(pub))
	accountID.RegisterAlias(name)

	peerID := lo.PanicOnErr(peer.IDFromPrivateKey(lo.PanicOnErr(p2pcrypto.UnmarshalEd25519PrivateKey(priv))))
	RegisterIDAlias(peerID, name)

	var validationBlockIssuer *BlockIssuer
	if validator {
		validationBlockIssuer = NewBlockIssuer(t, name, keyManager, accountID, validator)
	} else {
		validationBlockIssuer = nil
	}

	return &Node{
		Testing: t,
		logger:  parentLogger.NewChildLogger(name),

		Name: name,

		Validator:  validationBlockIssuer,
		KeyManager: keyManager,

		PeerID: peerID,

		Partition: partition,
		Endpoint:  net.JoinWithEndpointID(peerID, partition),
		Workers:   workerpool.NewGroup(name),

		enableEngineLogging: true,

		attachedBlocks:           make([]*blocks.Block, 0),
		invalidTransactionEvents: make(map[iotago.SignedTransactionID]InvalidSignedTransactionEvent),
	}
}

func (n *Node) SetCurrentSlot(slot iotago.SlotIndex) {
	n.currentSlot = slot
}

func (n *Node) IsValidator() bool {
	return n.Validator != nil
}

func (n *Node) Initialize(failOnBlockFiltered bool, opts ...options.Option[protocol.Protocol]) {
	n.Protocol = protocol.New(
		n.logger,
		n.Workers.CreateGroup("Protocol"),
		n.Endpoint,
		opts...,
	)

	n.hookEvents()

	if n.enableEngineLogging {
		n.hookLogging(failOnBlockFiltered)
	}

	n.ctx, n.ctxCancel = context.WithCancel(context.Background())

	started := make(chan struct{}, 1)

	n.Protocol.Initialized.OnTrigger(func() {
		close(started)
	})

	go func() {
		if err := n.Protocol.Run(n.ctx); err != nil {
			fmt.Printf("%s > Run finished with error: %s\n", n.Name, err.Error())
		}
	}()

	<-started
}

func (n *Node) hookEvents() {
	n.Protocol.Chains.HeaviestAttestedCandidate.OnUpdate(func(_ *protocol.Chain, heaviestAttestedCandidate *protocol.Chain) {
		if heaviestAttestedCandidate != nil {
			n.forkDetectedCount.Add(1)

			heaviestAttestedCandidate.Engine.OnUpdate(func(prevEngine *engine.Engine, newEngine *engine.Engine) {
				n.candidateEngineActivatedCount.Add(1)
			})
		}
	})

	n.Protocol.Chains.Main.OnUpdate(func(prevChain *protocol.Chain, newChain *protocol.Chain) {
		if prevChain != nil {
			n.mainEngineSwitchedCount.Add(1)
		}
	})

	n.Protocol.Events.Engine.PostSolidFilter.BlockFiltered.Hook(func(event *postsolidfilter.BlockFilteredEvent) {
		n.mutex.Lock()
		defer n.mutex.Unlock()

		n.filteredBlockEvents = append(n.filteredBlockEvents, event)
	})

	n.Protocol.Engines.Main.Get().Ledger.MemPool().OnSignedTransactionAttached(
		func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
			signedTxID := signedTransactionMetadata.ID()

			signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
				n.mutex.Lock()
				defer n.mutex.Unlock()

				n.invalidTransactionEvents[signedTxID] = InvalidSignedTransactionEvent{
					Metadata: signedTransactionMetadata,
					Error:    err,
				}
			})

			transactionMetadata := signedTransactionMetadata.TransactionMetadata()

			transactionMetadata.OnInvalid(func(err error) {
				n.mutex.Lock()
				defer n.mutex.Unlock()

				n.invalidTransactionEvents[signedTxID] = InvalidSignedTransactionEvent{
					Metadata: signedTransactionMetadata,
					Error:    err,
				}
			})
		})
}

func (n *Node) hookLogging(failOnBlockFiltered bool) {
	n.Protocol.Chains.WithElements(func(chain *protocol.Chain) (teardown func()) {
		return chain.Engine.OnUpdate(func(_ *engine.Engine, newEngine *engine.Engine) {
			if newEngine != nil {
				n.attachEngineLogs(failOnBlockFiltered, newEngine)
			}
		})
	})
}

func (n *Node) attachEngineLogsWithName(failOnBlockFiltered bool, instance *engine.Engine) {
	events := instance.Events

	events.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockDAG.BlockAttached", "block", block.ID())

		n.mutex.Lock()
		defer n.mutex.Unlock()
		n.attachedBlocks = append(n.attachedBlocks, block)
	})

	events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockDAG.BlockSolid", "block", block.ID())
	})

	events.BlockDAG.BlockInvalid.Hook(func(block *blocks.Block, err error) {
		instance.LogTrace("BlockDAG.BlockInvalid", "block", block.ID(), "err", err)
	})

	events.BlockDAG.BlockMissing.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockDAG.BlockMissing", "block", block.ID())
	})

	events.BlockDAG.MissingBlockAttached.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockDAG.MissingBlockAttached", "block", block.ID())
	})

	events.SeatManager.BlockProcessed.Hook(func(block *blocks.Block) {
		instance.LogTrace("SeatManager.BlockProcessed", "block", block.ID())
	})

	events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
		instance.LogTrace("Booker.BlockBooked", "block", block.ID())
	})

	events.Booker.BlockInvalid.Hook(func(block *blocks.Block, err error) {
		instance.LogTrace("Booker.BlockInvalid", "block", block.ID(), "err", err)
	})

	events.Booker.TransactionInvalid.Hook(func(metadata mempool.TransactionMetadata, err error) {
		instance.LogTrace("Booker.TransactionInvalid", "tx", metadata.ID(), "err", err)
	})

	events.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
		instance.LogTrace("Scheduler.BlockScheduled", "block", block.ID())
	})

	events.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
		instance.LogTrace("Scheduler.BlockEnqueued", "block", block.ID())
	})

	events.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
		instance.LogTrace("Scheduler.BlockSkipped", "block", block.ID())
	})

	events.Scheduler.BlockDropped.Hook(func(block *blocks.Block, err error) {
		instance.LogTrace("Scheduler.BlockDropped", "block", block.ID(), "err", err)
	})

	events.Clock.AcceptedTimeUpdated.Hook(func(newTime time.Time) {
		instance.LogTrace("Clock.AcceptedTimeUpdated", "time", newTime, "slot", instance.LatestAPI().TimeProvider().SlotFromTime(newTime))
	})

	events.Clock.ConfirmedTimeUpdated.Hook(func(newTime time.Time) {
		instance.LogTrace("Clock.ConfirmedTimeUpdated", "time", newTime, "slot", instance.LatestAPI().TimeProvider().SlotFromTime(newTime))
	})

	events.PreSolidFilter.BlockPreAllowed.Hook(func(block *model.Block) {
		instance.LogTrace("PreSolidFilter.BlockPreAllowed", "block", block.ID())
	})

	events.PreSolidFilter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
		instance.LogTrace("PreSolidFilter.BlockPreFiltered", "block", event.Block.ID(), "err", event.Reason)

		if failOnBlockFiltered {
			n.Testing.Fatal("no blocks should be prefiltered", "block", event.Block.ID(), "err", event.Reason)
		}
	})

	events.PostSolidFilter.BlockAllowed.Hook(func(block *blocks.Block) {
		instance.LogTrace("PostSolidFilter.BlockAllowed", "block", block.ID())
	})

	events.PostSolidFilter.BlockFiltered.Hook(func(event *postsolidfilter.BlockFilteredEvent) {
		instance.LogTrace("PostSolidFilter.BlockFiltered", "block", event.Block.ID(), "err", event.Reason)

		if failOnBlockFiltered {
			n.Testing.Fatal("no blocks should be filtered")
		}

		n.mutex.Lock()
		defer n.mutex.Unlock()
		n.filteredBlockEvents = append(n.filteredBlockEvents, event)
	})

	events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		instance.LogTrace("BlockRequester.Tick", "block", blockID)
	})

	events.BlockProcessed.Hook(func(blockID iotago.BlockID) {
		instance.LogTrace("BlockProcessed", "block", blockID)
	})

	events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		var acceptedBlocks iotago.BlockIDs
		err := details.AcceptedBlocks.Stream(func(id iotago.BlockID) error {
			acceptedBlocks = append(acceptedBlocks, id)
			return nil
		})
		require.NoError(n.Testing, err)

		rootsStorage, err := instance.Storage.Roots(details.Commitment.ID().Slot())
		require.NoError(n.Testing, err, "roots storage for slot %d not found", details.Commitment.Slot())
		roots, exists, err := rootsStorage.Load(details.Commitment.ID())
		require.NoError(n.Testing, err)
		require.True(n.Testing, exists)

		attestationBlockIDs := make([]iotago.BlockID, 0)
		tree, err := instance.Attestations.GetMap(details.Commitment.Slot())
		if err == nil {
			err = tree.Stream(func(key iotago.AccountID, value *iotago.Attestation) error {
				attestationBlockIDs = append(attestationBlockIDs, lo.PanicOnErr(value.BlockID()))
				return nil
			})
			require.NoError(n.Testing, err)
		}

		instance.LogTrace("NotarizationManager.SlotCommitted", "commitment", details.Commitment.ID(), "acceptedBlocks", acceptedBlocks, "roots", roots, "attestations", attestationBlockIDs)
	})

	events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		instance.LogTrace("NotarizationManager.LatestCommitmentUpdated", "commitment", commitment.ID())
	})

	events.BlockGadget.BlockPreAccepted.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockGadget.BlockPreAccepted", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
	})

	events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockGadget.BlockAccepted", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
	})

	events.BlockGadget.BlockPreConfirmed.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockGadget.BlockPreConfirmed", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
	})

	events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
		instance.LogTrace("BlockGadget.BlockConfirmed", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
	})

	events.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
		instance.LogTrace("SlotGadget.SlotFinalized", "slot", slot)
	})

	events.SeatManager.OnlineCommitteeSeatAdded.Hook(func(seat account.SeatIndex, accountID iotago.AccountID) {
		instance.LogTrace("SybilProtection.OnlineCommitteeSeatAdded", "seat", seat, "accountID", accountID)
	})

	events.SeatManager.OnlineCommitteeSeatRemoved.Hook(func(seat account.SeatIndex) {
		instance.LogTrace("SybilProtection.OnlineCommitteeSeatRemoved", "seat", seat)
	})

	events.SybilProtection.CommitteeSelected.Hook(func(committee *account.Accounts, epoch iotago.EpochIndex) {
		instance.LogTrace("SybilProtection.CommitteeSelected", "epoch", epoch, "committee", committee.IDs())
	})

	events.SpendDAG.SpenderCreated.Hook(func(conflictID iotago.TransactionID) {
		instance.LogTrace("SpendDAG.SpenderCreated", "conflictID", conflictID)
	})

	events.SpendDAG.SpenderEvicted.Hook(func(conflictID iotago.TransactionID) {
		instance.LogTrace("SpendDAG.SpenderEvicted", "conflictID", conflictID)
	})

	events.SpendDAG.SpenderRejected.Hook(func(conflictID iotago.TransactionID) {
		instance.LogTrace("SpendDAG.SpenderRejected", "conflictID", conflictID)
	})

	events.SpendDAG.SpenderAccepted.Hook(func(conflictID iotago.TransactionID) {
		instance.LogTrace("SpendDAG.SpenderAccepted", "conflictID", conflictID)
	})

	instance.Ledger.MemPool().OnSignedTransactionAttached(
		func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
			signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
				instance.LogTrace("MemPool.SignedTransactionSignaturesInvalid", "signedTx", signedTransactionMetadata.ID(), "tx", signedTransactionMetadata.TransactionMetadata().ID(), "err", err)
			})
		},
	)

	instance.Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
		instance.LogTrace("Ledger.TransactionAttached", "tx", transactionMetadata.ID())

		transactionMetadata.OnSolid(func() {
			instance.LogTrace("MemPool.TransactionSolid", "tx", transactionMetadata.ID())
		})

		transactionMetadata.OnExecuted(func() {
			instance.LogTrace("MemPool.TransactionExecuted", "tx", transactionMetadata.ID())
		})

		transactionMetadata.OnBooked(func() {
			instance.LogTrace("MemPool.TransactionBooked", "tx", transactionMetadata.ID())
		})

		transactionMetadata.OnConflicting(func() {
			instance.LogTrace("MemPool.TransactionConflicting", "tx", transactionMetadata.ID())
		})

		transactionMetadata.OnAccepted(func() {
			instance.LogTrace("MemPool.TransactionAccepted", "tx", transactionMetadata.ID())
		})

		transactionMetadata.OnRejected(func() {
			instance.LogTrace("MemPool.TransactionRejected", "tx", transactionMetadata.ID())
		})

		transactionMetadata.OnInvalid(func(err error) {
			instance.LogTrace("MemPool.TransactionInvalid", "tx", transactionMetadata.ID(), "err", err)
		})

		transactionMetadata.OnOrphanedSlotUpdated(func(slot iotago.SlotIndex) {
			instance.LogTrace("MemPool.TransactionOrphanedSlotUpdated", "tx", transactionMetadata.ID(), "slot", slot)
		})

		transactionMetadata.OnCommittedSlotUpdated(func(slot iotago.SlotIndex) {
			instance.LogTrace("MemPool.TransactionCommittedSlotUpdated", "tx", transactionMetadata.ID(), "slot", slot)
		})

		transactionMetadata.OnPending(func() {
			instance.LogTrace("MemPool.TransactionPending", "tx", transactionMetadata.ID())
		})
	})
}

func (n *Node) attachEngineLogs(failOnBlockFiltered bool, instance *engine.Engine) {
	n.attachEngineLogsWithName(failOnBlockFiltered, instance)
}

func (n *Node) Wait() {
	n.Workers.WaitChildren()
}

func (n *Node) Shutdown() {
	stopped := make(chan struct{}, 1)

	if n.Protocol != nil {
		n.Protocol.Stopped.OnTrigger(func() {
			close(stopped)
		})
	} else {
		close(stopped)
	}

	if n.ctxCancel != nil {
		n.ctxCancel()
	}

	<-stopped
	n.logger.UnsubscribeFromParentLogger()
}

func (n *Node) ProtocolParametersHash() iotago.Identifier {
	if n.protocolParametersHash == iotago.EmptyIdentifier {
		return lo.PanicOnErr(n.Protocol.LatestAPI().ProtocolParameters().Hash())
	}

	return n.protocolParametersHash
}

func (n *Node) SetProtocolParametersHash(hash iotago.Identifier) {
	n.protocolParametersHash = hash
}

func (n *Node) HighestSupportedVersion() iotago.Version {
	if n.highestSupportedVersion == 0 {
		return n.Protocol.LatestAPI().Version()
	}

	return n.highestSupportedVersion
}

func (n *Node) SetHighestSupportedVersion(version iotago.Version) {
	n.highestSupportedVersion = version
}

func (n *Node) ForkDetectedCount() int {
	return int(n.forkDetectedCount.Load())
}

func (n *Node) CandidateEngineActivatedCount() int {
	return int(n.candidateEngineActivatedCount.Load())
}

func (n *Node) FilteredBlocks() []*postsolidfilter.BlockFilteredEvent {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	return n.filteredBlockEvents
}

func (n *Node) TransactionFailure(txID iotago.SignedTransactionID) (InvalidSignedTransactionEvent, bool) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	event, exists := n.invalidTransactionEvents[txID]

	return event, exists
}

func (n *Node) MainEngineSwitchedCount() int {
	return int(n.mainEngineSwitchedCount.Load())
}

func (n *Node) AttachedBlocks() []*blocks.Block {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	return n.attachedBlocks
}

func (n *Node) IssueValidationBlock(ctx context.Context, alias string, opts ...options.Option[ValidationBlockParams]) *blocks.Block {
	if n.Validator == nil {
		panic("node is not a validator")
	}

	return n.Validator.IssueValidationBlock(ctx, alias, n, opts...)
}
