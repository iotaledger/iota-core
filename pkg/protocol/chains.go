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
	HeaviestClaimedCandidate reactive.Variable[*Chain]

	// HeaviestAttestedCandidate contains the candidate chain with the heaviest weight as checked by attestations. The chain has not been instantiated into an engine yet.
	HeaviestAttestedCandidate reactive.Variable[*Chain]

	// HeaviestVerifiedCandidate contains the candidate chain with the heaviest verified weight.
	HeaviestVerifiedCandidate reactive.Variable[*Chain]

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
		Set:                       reactive.NewSet[*Chain](),
		Main:                      reactive.NewVariable[*Chain](),
		HeaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		HeaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		HeaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
		LatestSeenSlot:            reactive.NewVariable[iotago.SlotIndex](increasing[iotago.SlotIndex]),
		protocol:                  protocol,
	}

	shutdown := lo.Batch(
		c.initLogger(protocol.NewChildLogger("Chains")),
		c.initChainSwitching(protocol.Options.ChainSwitchingThreshold),

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
func (c *Chains) initLogger(logger log.Logger, shutdownLogger func()) (shutdown func()) {
	c.Logger = logger

	return lo.Batch(
		c.Main.LogUpdates(c, log.LevelTrace, "Main", (*Chain).LogName),
		c.HeaviestClaimedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestClaimedCandidate", (*Chain).LogName),
		c.HeaviestAttestedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestAttestedCandidate", (*Chain).LogName),
		c.HeaviestVerifiedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestVerifiedCandidate", (*Chain).LogName),

		shutdownLogger,
	)
}

// initChainSwitching initializes the chain switching logic.
func (c *Chains) initChainSwitching(chainSwitchingThreshold iotago.SlotIndex) (shutdown func()) {
	mainChain := c.newChain()
	mainChain.StartEngine.Set(true)

	c.Main.Set(mainChain)

	return lo.Batch(
		c.HeaviestClaimedCandidate.WithNonEmptyValue(func(heaviestClaimedCandidate *Chain) (shutdown func()) {
			return heaviestClaimedCandidate.RequestAttestations.ToggleValue(true)
		}),

		c.HeaviestAttestedCandidate.WithNonEmptyValue(func(heaviestAttestedCandidate *Chain) (shutdown func()) {
			return heaviestAttestedCandidate.StartEngine.ToggleValue(true)
		}),

		c.HeaviestVerifiedCandidate.WithNonEmptyValue(func(heaviestVerifiedCandidate *Chain) (shutdown func()) {
			// only switch to the heaviest chain if the latest produced commitment is enough slots away from the forking point.
			chainSwitchingCondition := func(_ *Commitment, latestProducedCommitment *Commitment) bool {
				forkingPoint := heaviestVerifiedCandidate.ForkingPoint.Get()

				return forkingPoint != nil && latestProducedCommitment != nil && (latestProducedCommitment.ID().Slot()-forkingPoint.ID().Slot()) > chainSwitchingThreshold
			}

			return heaviestVerifiedCandidate.LatestProducedCommitment.OnUpdateOnce(func(_ *Commitment, latestProducedCommitment *Commitment) {
				c.Main.Set(heaviestVerifiedCandidate)
			}, chainSwitchingCondition)
		}),

		c.WithElements(func(candidateChain *Chain) (shutdown func()) {
			return lo.Batch(
				c.initHeaviestCandidateTracking(c.HeaviestClaimedCandidate, (*Chain).claimedWeight, candidateChain),
				c.initHeaviestCandidateTracking(c.HeaviestVerifiedCandidate, (*Chain).verifiedWeight, candidateChain),
				c.initHeaviestCandidateTracking(c.HeaviestAttestedCandidate, (*Chain).attestedWeight, candidateChain),
			)
		}),
	)
}

// initHeaviestCandidateTracking initializes the tracking of the heaviest candidates according to the given parameters.
func (c *Chains) initHeaviestCandidateTracking(candidateVar reactive.Variable[*Chain], weightVar func(*Chain) reactive.Variable[uint64], newCandidate *Chain) (unsubscribe func()) {
	return weightVar(newCandidate).OnUpdate(func(_ uint64, newWeight uint64) {
		// abort if the candidate is not heavier than the main chain.
		if mainChain := c.Main.Get(); newCandidate == mainChain || newWeight <= mainChain.VerifiedWeight.Get() {
			return
		}

		// atomically replace the existing candidate if the new one is heavier.
		candidateVar.Compute(func(currentCandidate *Chain) *Chain {
			if currentCandidate != nil && !currentCandidate.IsEvicted.WasTriggered() && newWeight <= weightVar(currentCandidate).Get() {
				return currentCandidate
			}

			return newCandidate
		})
	}, true)
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
