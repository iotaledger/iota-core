package protocol

import (
	"cmp"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Chains ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Chains is a subcomponent of the protocol that exposes the chains that are managed by the protocol and that implements
// the chain switching logic.
type Chains struct {
	// Set contains all non-evicted chains that are managed by the protocol.
	reactive.Set[*Chain]

	// Main contains the main chain.
	Main reactive.Variable[*Chain]

	// HeaviestClaimedCandidate contains the candidate chain with the heaviest claimed weight according to its latest commitment. The weight has neither been checked via attestations nor verified by downloading all data.
	HeaviestClaimedCandidate *ChainsCandidate

	// HeaviestAttestedCandidate contains the candidate chain with the heaviest weight as checked by attestations. The chain has not been instantiated into an engine yet.
	HeaviestAttestedCandidate *ChainsCandidate

	// HeaviestVerifiedCandidate contains the candidate chain with the heaviest verified weight, meaning the chain has been instantiated into an engine and the commitments have been produced by the engine itself.
	HeaviestVerifiedCandidate *ChainsCandidate

	// LatestSeenSlot contains the slot of the latest commitment of any received block.
	LatestSeenSlot reactive.Variable[iotago.SlotIndex]

	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// Logger contains a reference to the logger that is used by this component.
	log.Logger
}

// newChains creates a new chains instance for the given protocol.
func newChains(protocol *Protocol) *Chains {
	c := &Chains{
		Set:            reactive.NewSet[*Chain](),
		Main:           reactive.NewVariable[*Chain](),
		LatestSeenSlot: reactive.NewVariable[iotago.SlotIndex](increasing[iotago.SlotIndex]),
		protocol:       protocol,
	}

	c.HeaviestClaimedCandidate = newChainsCandidate(c, (*Commitment).cumulativeWeight)
	c.HeaviestAttestedCandidate = newChainsCandidate(c, (*Commitment).cumulativeAttestedWeight)
	c.HeaviestVerifiedCandidate = newChainsCandidate(c, (*Commitment).cumulativeVerifiedWeight)

	c.WithElements(func(chain *Chain) (teardown func()) {
		return chain.Engine.OnUpdate(func(_ *engine.Engine, newEngine *engine.Engine) {
			if newEngine != nil {
				newEngine.OnLogLevelActive(log.LevelTrace, func() (shutdown func()) {
					return attachEngineLogs(newEngine)
				})
			}
		})
	})

	shutdown := lo.BatchReverse(
		c.initLogger(protocol.NewChildLogger("Chains")),
		c.initChainSwitching(),

		protocol.ConstructedEvent().WithNonEmptyValue(func(_ bool) (shutdown func()) {
			return c.deriveLatestSeenSlot(protocol)
		}),
	)

	protocol.ShutdownEvent().OnTrigger(shutdown)

	return c
}

func attachEngineLogs(instance *engine.Engine) func() {
	events := instance.Events

	return lo.BatchReverse(
		events.BlockDAG.BlockAppended.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockDAG.BlockAppended", "block", block.ID())
		}).Unhook,

		events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockDAG.BlockSolid", "block", block.ID())
		}).Unhook,

		events.BlockDAG.BlockInvalid.Hook(func(block *blocks.Block, err error) {
			instance.LogTrace("BlockDAG.BlockInvalid", "block", block.ID(), "err", err)
		}).Unhook,

		events.BlockDAG.BlockMissing.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockDAG.BlockMissing", "block", block.ID())
		}).Unhook,

		events.BlockDAG.MissingBlockAppended.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockDAG.MissingBlockAppended", "block", block.ID())
		}).Unhook,

		events.SeatManager.BlockProcessed.Hook(func(block *blocks.Block) {
			instance.LogTrace("SeatManager.BlockProcessed", "block", block.ID())
		}).Unhook,

		events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
			instance.LogTrace("Booker.BlockBooked", "block", block.ID())
		}).Unhook,

		events.Booker.BlockInvalid.Hook(func(block *blocks.Block, err error) {
			instance.LogTrace("Booker.BlockInvalid", "block", block.ID(), "err", err)
		}).Unhook,

		events.Booker.TransactionInvalid.Hook(func(metadata mempool.TransactionMetadata, err error) {
			instance.LogTrace("Booker.TransactionInvalid", "tx", metadata.ID(), "err", err)
		}).Unhook,

		events.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
			instance.LogTrace("Scheduler.BlockScheduled", "block", block.ID())
		}).Unhook,

		events.Scheduler.BlockEnqueued.Hook(func(block *blocks.Block) {
			instance.LogTrace("Scheduler.BlockEnqueued", "block", block.ID())
		}).Unhook,

		events.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
			instance.LogTrace("Scheduler.BlockSkipped", "block", block.ID())
		}).Unhook,

		events.Scheduler.BlockDropped.Hook(func(block *blocks.Block, err error) {
			instance.LogTrace("Scheduler.BlockDropped", "block", block.ID(), "err", err)
		}).Unhook,

		events.Clock.AcceptedTimeUpdated.Hook(func(newTime time.Time) {
			instance.LogTrace("Clock.AcceptedTimeUpdated", "time", newTime, "slot", instance.LatestAPI().TimeProvider().SlotFromTime(newTime))
		}).Unhook,

		events.Clock.ConfirmedTimeUpdated.Hook(func(newTime time.Time) {
			instance.LogTrace("Clock.ConfirmedTimeUpdated", "time", newTime, "slot", instance.LatestAPI().TimeProvider().SlotFromTime(newTime))
		}).Unhook,

		events.PreSolidFilter.BlockPreAllowed.Hook(func(block *model.Block) {
			instance.LogTrace("PreSolidFilter.BlockPreAllowed", "block", block.ID())
		}).Unhook,

		events.PreSolidFilter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
			instance.LogTrace("PreSolidFilter.BlockPreFiltered", "block", event.Block.ID(), "err", event.Reason)
		}).Unhook,

		events.PostSolidFilter.BlockAllowed.Hook(func(block *blocks.Block) {
			instance.LogTrace("PostSolidFilter.BlockAllowed", "block", block.ID())
		}).Unhook,

		events.PostSolidFilter.BlockFiltered.Hook(func(event *postsolidfilter.BlockFilteredEvent) {
			instance.LogTrace("PostSolidFilter.BlockFiltered", "block", event.Block.ID(), "err", event.Reason)
		}).Unhook,

		events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
			instance.LogTrace("BlockRequester.Tick", "block", blockID)
		}).Unhook,

		events.BlockProcessed.Hook(func(blockID iotago.BlockID) {
			instance.LogTrace("BlockProcessed", "block", blockID)
		}).Unhook,

		events.Retainer.BlockRetained.Hook(func(block *blocks.Block) {
			instance.LogTrace("Retainer.BlockRetained", "block", block.ID())
		}).Unhook,

		events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
			instance.LogTrace("NotarizationManager.SlotCommitted", "commitment", details.Commitment.ID(), "acceptedBlocks count", details.AcceptedBlocks.Size(), "accepted transactions", len(details.Mutations))
		}).Unhook,

		events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
			instance.LogTrace("NotarizationManager.LatestCommitmentUpdated", "commitment", commitment.ID())
		}).Unhook,

		events.BlockGadget.BlockPreAccepted.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockGadget.BlockPreAccepted", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
		}).Unhook,

		events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockGadget.BlockAccepted", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
		}).Unhook,

		events.BlockGadget.BlockPreConfirmed.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockGadget.BlockPreConfirmed", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
		}).Unhook,

		events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
			instance.LogTrace("BlockGadget.BlockConfirmed", "block", block.ID(), "slotCommitmentID", block.ProtocolBlock().Header.SlotCommitmentID)
		}).Unhook,

		events.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
			instance.LogTrace("SlotGadget.SlotFinalized", "slot", slot)
		}).Unhook,

		events.SeatManager.OnlineCommitteeSeatAdded.Hook(func(seat account.SeatIndex, accountID iotago.AccountID) {
			instance.LogTrace("SybilProtection.OnlineCommitteeSeatAdded", "seat", seat, "accountID", accountID)
		}).Unhook,

		events.SeatManager.OnlineCommitteeSeatRemoved.Hook(func(seat account.SeatIndex) {
			instance.LogTrace("SybilProtection.OnlineCommitteeSeatRemoved", "seat", seat)
		}).Unhook,

		events.SybilProtection.CommitteeSelected.Hook(func(committee *account.SeatedAccounts, epoch iotago.EpochIndex) {
			instance.LogTrace("SybilProtection.CommitteeSelected", "epoch", epoch, "committee", committee.IDs())
		}).Unhook,

		events.SpendDAG.SpenderCreated.Hook(func(conflictID iotago.TransactionID) {
			instance.LogTrace("SpendDAG.SpenderCreated", "conflictID", conflictID)
		}).Unhook,

		events.SpendDAG.SpenderEvicted.Hook(func(conflictID iotago.TransactionID) {
			instance.LogTrace("SpendDAG.SpenderEvicted", "conflictID", conflictID)
		}).Unhook,

		events.SpendDAG.SpenderRejected.Hook(func(conflictID iotago.TransactionID) {
			instance.LogTrace("SpendDAG.SpenderRejected", "conflictID", conflictID)
		}).Unhook,

		events.SpendDAG.SpenderAccepted.Hook(func(conflictID iotago.TransactionID) {
			instance.LogTrace("SpendDAG.SpenderAccepted", "conflictID", conflictID)
		}).Unhook,

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

			transactionMetadata.OnEvicted(func() {
				instance.LogTrace("MemPool.TransactionEvicted", "tx", transactionMetadata.ID())
			})
		}).Unhook,

		instance.Ledger.MemPool().OnSignedTransactionAttached(
			func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
				signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
					instance.LogTrace("MemPool.SignedTransactionSignaturesInvalid", "signedTx", signedTransactionMetadata.ID(), "tx", signedTransactionMetadata.TransactionMetadata().ID(), "err", err)
				})
			},
		).Unhook,
	)
}

// WithInitializedEngines is a reactive selector that executes the given callback for each managed chain that
// initialized its engine.
func (c *Chains) WithInitializedEngines(callback func(chain *Chain, engine *engine.Engine) (shutdown func())) (shutdown func()) {
	return c.WithElements(func(chain *Chain) (shutdown func()) {
		return chain.WithInitializedEngine(func(engine *engine.Engine) (shutdown func()) {
			return callback(chain, engine)
		})
	})
}

// initLogger initializes the logger for this component.
func (c *Chains) initLogger(logger log.Logger) (shutdown func()) {
	c.Logger = logger

	return lo.BatchReverse(
		c.Main.LogUpdates(c, log.LevelTrace, "Main", (*Chain).LogName),
		c.HeaviestClaimedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestClaimedCandidate", (*Chain).LogName),
		c.HeaviestAttestedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestAttestedCandidate", (*Chain).LogName),
		c.HeaviestVerifiedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestVerifiedCandidate", (*Chain).LogName),

		logger.UnsubscribeFromParentLogger,
	)
}

// initChainSwitching initializes the chain switching logic.
func (c *Chains) initChainSwitching() (shutdown func()) {
	mainChain := c.newChain()
	mainChain.StartEngine.Set(true)

	c.Main.Set(mainChain)

	return lo.BatchReverse(
		c.HeaviestClaimedCandidate.WithNonEmptyValue(func(heaviestClaimedCandidate *Chain) (shutdown func()) {
			return heaviestClaimedCandidate.RequestAttestations.ToggleValue(true)
		}),

		c.HeaviestAttestedCandidate.OnUpdate(func(_ *Chain, heaviestAttestedCandidate *Chain) {
			if heaviestAttestedCandidate != nil {
				heaviestAttestedCandidate.StartEngine.Set(true)
			}
		}),

		c.HeaviestVerifiedCandidate.OnUpdate(func(_ *Chain, heaviestVerifiedCandidate *Chain) {
			if heaviestVerifiedCandidate != nil {
				c.Main.Set(heaviestVerifiedCandidate)
			}
		}),

		c.WithElements(c.trackHeaviestCandidates),
		c.LatestSeenSlot.WithNonEmptyValue(c.updateMeasuredSlot),
	)
}

func (c *Chains) trackHeaviestCandidates(chain *Chain) (teardown func()) {
	return chain.LatestCommitment.OnUpdate(func(_ *Commitment, latestCommitment *Commitment) {
		targetSlot := latestCommitment.ID().Index()

		if evictionEvent := c.protocol.EvictionEvent(targetSlot); !evictionEvent.WasTriggered() {
			c.HeaviestClaimedCandidate.registerCommitment(targetSlot, latestCommitment, evictionEvent)

			latestCommitment.IsAttested.OnTrigger(func() {
				c.HeaviestAttestedCandidate.registerCommitment(targetSlot, latestCommitment, evictionEvent)
			})

			latestCommitment.IsVerified.OnTrigger(func() {
				c.HeaviestVerifiedCandidate.registerCommitment(targetSlot, latestCommitment, evictionEvent)
			})
		}
	})
}

func (c *Chains) updateMeasuredSlot(latestSeenSlot iotago.SlotIndex) (teardown func()) {
	measuredSlot := latestSeenSlot - chainSwitchingMeasurementOffset

	return lo.BatchReverse(
		c.HeaviestClaimedCandidate.measureAt(measuredSlot),
		c.HeaviestAttestedCandidate.measureAt(measuredSlot),
		c.HeaviestVerifiedCandidate.measureAt(measuredSlot),
	)
}

// deriveLatestSeenSlot derives the latest seen slot from the protocol.
func (c *Chains) deriveLatestSeenSlot(protocol *Protocol) func() {
	//nolint:revive
	return protocol.Engines.Main.WithNonEmptyValue(func(mainEngine *engine.Engine) (shutdown func()) {
		return lo.BatchReverse(
			c.WithInitializedEngines(func(_ *Chain, engine *engine.Engine) (shutdown func()) {
				return engine.LatestCommitment.OnUpdate(func(_ *model.Commitment, latestCommitment *model.Commitment) {
					c.LatestSeenSlot.Set(latestCommitment.Slot())
				})
			}),

			protocol.Network.OnBlockReceived(func(block *model.Block, _ peer.ID) {
				c.LatestSeenSlot.Set(block.ProtocolBlock().Header.SlotCommitmentID.Slot())
			}),
		)
	})
}

// newChain creates a new chain instance and adds it to the set of chains.
func (c *Chains) newChain() *Chain {
	chain := newChain(c)
	if c.Add(chain) {
		chain.IsEvicted.OnTrigger(func() { c.Delete(chain) })
	}

	return chain
}

// increasing is a generic function that returns the maximum of the two given values.
func increasing[T cmp.Ordered](currentValue T, newValue T) T {
	return max(currentValue, newValue)
}

const chainSwitchingMeasurementOffset iotago.SlotIndex = 1

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region ChainsCandidate //////////////////////////////////////////////////////////////////////////////////////////////

// ChainsCandidate implements a wrapper for the logic of tracking the heaviest candidate of all Chains in respect to
// some monitored weight variable.
type ChainsCandidate struct {
	// Variable contains the heaviest chain candidate.
	reactive.Variable[*Chain]

	// chains contains a reference to the Chains instance that this candidate belongs to.
	chains *Chains

	// weightVariable contains the weight variable that is used to determine the heaviest chain candidate.
	weightVariable func(element *Commitment) reactive.Variable[uint64]

	// sortedCommitmentsBySlot contains the sorted commitments for each slot.
	sortedCommitmentsBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]
}

// newChainsCandidate creates a new heaviest chain candidate.
func newChainsCandidate(chains *Chains, weightVariable func(element *Commitment) reactive.Variable[uint64]) *ChainsCandidate {
	return &ChainsCandidate{
		Variable:                reactive.NewVariable[*Chain](),
		chains:                  chains,
		sortedCommitmentsBySlot: shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		weightVariable:          weightVariable,
	}
}

// measureAt measures the heaviest chain candidate at the given slot and updates the variable as soon as the threshold
// of chainSwitchingThreshold slots with the same heaviest chain in respect to the given slot is reached.
func (c *ChainsCandidate) measureAt(slot iotago.SlotIndex) (teardown func()) {
	// sanitize protocol parameters
	chainSwitchingThreshold := c.chains.protocol.APIForSlot(slot).ProtocolParameters().ChainSwitchingThreshold()
	if slot < iotago.SlotIndex(chainSwitchingThreshold) {
		return nil
	}

	// get the sorted commitments for the given slot
	sortedCommitments := c.sortedCommitments(slot, c.chains.protocol.EvictionEvent(slot))

	// make sure the heaviest commitment was the heaviest for the last chainSwitchingThreshold slots before we update
	return sortedCommitments.HeaviestElement().WithNonEmptyValue(func(heaviestCommitment *Commitment) (teardown func()) {
		return c.weightVariable(heaviestCommitment).WithValue(func(candidateWeight uint64) (teardown func()) {
			heaviestChain := heaviestCommitment.Chain.Get()

			// abort if the heaviest commitment is the main chain or main chain is heavier
			if mainChain := c.chains.Main.Get(); heaviestChain == mainChain {
				return nil
			} else if mainChain.CumulativeVerifiedWeightAt(heaviestCommitment.Slot()) > candidateWeight {
				return nil
			}

			// create counter for the number of slots with the same chain
			slotsWithSameChain := reactive.NewCounter[*Commitment](func(commitment *Commitment) bool {
				return commitment.Chain.Get() == heaviestChain
			})

			// reactively counts the number of slots with the same chain
			var teardownMonitoringFunctions []func()
			for i := uint8(1); i < chainSwitchingThreshold; i++ {
				if earlierCommitments, earlierCommitmentsExist := c.sortedCommitmentsBySlot.Get(slot - iotago.SlotIndex(i)); earlierCommitmentsExist {
					teardownMonitoringFunctions = append(teardownMonitoringFunctions, slotsWithSameChain.Monitor(earlierCommitments.HeaviestElement()))
				}
			}

			// reactively update the value in respect to the reached threshold
			teardownUpdates := slotsWithSameChain.OnUpdate(func(_ int, slotsWithSameChain int) {
				if slotsWithSameChain >= int(chainSwitchingThreshold)-1 {
					c.Set(heaviestChain)
				} else {
					c.Set(nil)
				}
			})

			// return all teardown functions
			return lo.BatchReverse(append(teardownMonitoringFunctions, teardownUpdates)...)
		})
	})
}

// registerCommitment registers the given commitment for the given slot, which makes it become part of the weight
// measurement process.
func (c *ChainsCandidate) registerCommitment(slot iotago.SlotIndex, commitment *Commitment, evictionEvent reactive.Event) {
	sortedCommitments := c.sortedCommitments(slot, evictionEvent)
	sortedCommitments.Add(commitment)
}

// sortedCommitments returns the sorted commitments for the given slot and creates a new sorted set if it doesn't exist.
func (c *ChainsCandidate) sortedCommitments(slot iotago.SlotIndex, evictionEvent reactive.Event) (sortedCommitments reactive.SortedSet[*Commitment]) {
	sortedCommitments, slotCreated := c.sortedCommitmentsBySlot.GetOrCreate(slot, func() reactive.SortedSet[*Commitment] {
		return reactive.NewSortedSet(c.weightVariable)
	})

	if slotCreated {
		evictionEvent.OnTrigger(func() { c.sortedCommitmentsBySlot.Delete(slot) })
	}

	return sortedCommitments
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
