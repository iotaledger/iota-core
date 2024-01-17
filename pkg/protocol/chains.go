package protocol

import (
	"cmp"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Chains is a subcomponent of the protocol that exposes the chains that are managed by the protocol and that implements
// the chain switching logic.
type Chains struct {
	// Set contains all non-evicted chains that are managed by the protocol.
	reactive.Set[*Chain]

	// Main contains the main chain.
	Main reactive.Variable[*Chain]

	// HeaviestClaimedCandidate contains the candidate chain with the heaviest claimed weight according to its latest commitment. The weight has neither been checked via attestations nor verified by downloading all data.
	HeaviestClaimedCandidate *HeaviestChainCandidate

	// HeaviestAttestedCandidate contains the candidate chain with the heaviest weight as checked by attestations. The chain has not been instantiated into an engine yet.
	HeaviestAttestedCandidate *HeaviestChainCandidate

	// HeaviestVerifiedCandidate contains the candidate chain with the heaviest verified weight, meaning the chain has been instantiated into an engine and the commitments have been produced by the engine itself.
	HeaviestVerifiedCandidate *HeaviestChainCandidate

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

	c.HeaviestClaimedCandidate = newHeaviestChainCandidate((*Commitment).weightAddr, c.Main)
	c.HeaviestAttestedCandidate = newHeaviestChainCandidate((*Commitment).weightAddr, c.Main)
	c.HeaviestVerifiedCandidate = newHeaviestChainCandidate((*Commitment).weightAddr, c.Main)

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
		c.Main.LogUpdates(c, log.LevelInfo, "Main", (*Chain).LogName),
		c.HeaviestClaimedCandidate.LogUpdates(c, log.LevelInfo, "HeaviestClaimedCandidate", (*Chain).LogName),
		c.HeaviestAttestedCandidate.LogUpdates(c, log.LevelInfo, "HeaviestAttestedCandidate", (*Chain).LogName),
		c.HeaviestVerifiedCandidate.LogUpdates(c, log.LevelInfo, "HeaviestVerifiedCandidate", (*Chain).LogName),

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
			heaviestAttestedCandidate.StartEngine.Set(true)
		}),

		c.HeaviestVerifiedCandidate.OnUpdate(func(_ *Chain, heaviestVerifiedCandidate *Chain) {
			c.Main.Set(heaviestVerifiedCandidate)
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
	return protocol.Engines.Main.WithNonEmptyValue(func(mainEngine *engine.Engine) (shutdown func()) {
		return lo.Batch(
			mainEngine.Initialized.OnTrigger(func() {
				c.LatestSeenSlot.Set(mainEngine.LatestCommitment.Get().Slot())
			}),

			protocol.Network.OnBlockReceived(func(block *model.Block, src peer.ID) {
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

const (
	chainSwitchingThreshold iotago.SlotIndex = 10

	chainSwitchingMeasurementOffset iotago.SlotIndex = 1
)
