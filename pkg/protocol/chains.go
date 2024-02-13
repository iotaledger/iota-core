package protocol

import (
	"cmp"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
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

	shutdown := lo.Batch(
		c.initLogger(protocol.NewChildLogger("Chains")),
		c.initChainSwitching(),

		protocol.Constructed.WithNonEmptyValue(func(_ bool) (shutdown func()) {
			return c.deriveLatestSeenSlot(protocol)
		}),
	)

	protocol.Shutdown.OnTrigger(shutdown)

	return c
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

	return lo.Batch(
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

	return lo.Batch(
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
			c.HeaviestAttestedCandidate.registerCommitment(targetSlot, latestCommitment, evictionEvent)
			c.HeaviestVerifiedCandidate.registerCommitment(targetSlot, latestCommitment, evictionEvent)
		}
	})
}

func (c *Chains) updateMeasuredSlot(latestSeenSlot iotago.SlotIndex) (teardown func()) {
	measuredSlot := latestSeenSlot - chainSwitchingMeasurementOffset

	return lo.Batch(
		c.HeaviestClaimedCandidate.measureAt(measuredSlot),
		c.HeaviestAttestedCandidate.measureAt(measuredSlot),
		c.HeaviestVerifiedCandidate.measureAt(measuredSlot),
	)
}

// deriveLatestSeenSlot derives the latest seen slot from the protocol.
func (c *Chains) deriveLatestSeenSlot(protocol *Protocol) func() {
	//nolint:revive
	return protocol.Engines.Main.WithNonEmptyValue(func(mainEngine *engine.Engine) (shutdown func()) {
		return lo.Batch(
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
		return
	}

	// abort if no commitment exists for this slot
	sortedCommitments, sortedCommitmentsExist := c.sortedCommitmentsBySlot.Get(slot)
	if !sortedCommitmentsExist {
		return
	}

	// make sure the heaviest commitment was the heaviest for the last chainSwitchingThreshold slots before we update
	return sortedCommitments.HeaviestElement().WithNonEmptyValue(func(heaviestCommitment *Commitment) (teardown func()) {
		// abort if the heaviest commitment is the main chain
		heaviestChain := heaviestCommitment.Chain.Get()
		if heaviestChain == c.chains.Main.Get() {
			return
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
		return lo.Batch(append(teardownMonitoringFunctions, teardownUpdates)...)
	})
}

// registerCommitment registers the given commitment for the given slot, which makes it become part of the weight
// measurement process.
func (c *ChainsCandidate) registerCommitment(slot iotago.SlotIndex, commitment *Commitment, evictionEvent reactive.Event) {
	sortedCommitments, slotCreated := c.sortedCommitmentsBySlot.GetOrCreate(slot, func() reactive.SortedSet[*Commitment] {
		return reactive.NewSortedSet(c.weightVariable)
	})

	if slotCreated {
		evictionEvent.OnTrigger(func() { c.sortedCommitmentsBySlot.Delete(slot) })
	}

	sortedCommitments.Add(commitment)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
