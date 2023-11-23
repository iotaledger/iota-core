package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Chains is a subcomponent of the protocol that manages the chains and the corresponding chain switching logic, and it
// exposes these chains as a reactive set together with endpoints for the main chain and the heaviest candidates.
type Chains struct {
	// Set contains all chains that are managed by the protocol.
	reactive.Set[*Chain]

	// Main is a reactive variable that contains the main chain.
	Main reactive.Variable[*Chain]

	// HeaviestClaimedCandidate is a reactive variable that contains the candidate chain with the heaviest claimed
	// weight.
	HeaviestClaimedCandidate  reactive.Variable[*Chain]
	HeaviestAttestedCandidate reactive.Variable[*Chain]
	HeaviestVerifiedCandidate reactive.Variable[*Chain]

	protocol *Protocol

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

func newChains(protocol *Protocol) *Chains {
	return (&Chains{
		Set:                       reactive.NewSet[*Chain](),
		Main:                      reactive.NewVariable[*Chain](),
		HeaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		HeaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		HeaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
		protocol:                  protocol,
	}).init(protocol)
}

func (c *Chains) init(protocol *Protocol) (self *Chains) {
	shutdown := lo.Batch(
		c.initLogging(protocol.NewChildLogger("Chains")),
		c.initMainChain(),
		c.initChainSwitching(protocol.Options.ChainSwitchingThreshold),
	)

	c.protocol.Shutdown.OnTrigger(shutdown)

	return c
}

func (c *Chains) initLogging(logger log.Logger, shutdownLogger func()) (teardown func()) {
	c.Logger = logger

	return lo.Batch(
		c.Main.LogUpdates(c, log.LevelTrace, "Main", (*Chain).LogName),
		c.HeaviestClaimedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestClaimedCandidate", (*Chain).LogName),
		c.HeaviestAttestedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestAttestedCandidate", (*Chain).LogName),
		c.HeaviestVerifiedCandidate.LogUpdates(c, log.LevelTrace, "HeaviestVerifiedCandidate", (*Chain).LogName),

		shutdownLogger,
	)
}

func (c *Chains) initMainChain() (teardown func()) {
	chainLogger, shutdownChainLogger := c.NewEntityLogger("")

	mainChain := newChain(chainLogger, shutdownChainLogger, c.protocol.LatestSeenSlot)
	mainChain.RequestBlocks.Set(true)

	c.Add(mainChain)

	return c.Main.ToggleValue(mainChain)
}

func (c *Chains) initChainSwitching(chainSwitchingThreshold iotago.SlotIndex) (teardown func()) {
	return lo.Batch(
		c.WithElements(func(candidateChain *Chain) (teardown func()) {
			return lo.Batch(
				c.trackHeaviestCandidate(c.HeaviestClaimedCandidate, (*Chain).claimedWeightVariable, candidateChain),
				c.trackHeaviestCandidate(c.HeaviestVerifiedCandidate, (*Chain).verifiedWeightVariable, candidateChain),
				c.trackHeaviestCandidate(c.HeaviestAttestedCandidate, (*Chain).attestedWeightVariable, candidateChain),
			)
		}),

		c.HeaviestClaimedCandidate.WithNonEmptyValue(func(heaviestClaimedCandidate *Chain) (teardown func()) {
			return heaviestClaimedCandidate.RequestAttestations.ToggleValue(true)
		}),

		c.HeaviestAttestedCandidate.WithNonEmptyValue(func(heaviestAttestedCandidate *Chain) (teardown func()) {
			return heaviestAttestedCandidate.RequestBlocks.ToggleValue(true)
		}),

		c.HeaviestVerifiedCandidate.WithNonEmptyValue(func(heaviestVerifiedCandidate *Chain) (teardown func()) {
			// only switch to the heaviest chain if the latest produced commitment is enough slots away from the forking point.
			chainSwitchingCondition := func(_ *Commitment, latestProducedCommitment *Commitment) bool {
				forkingPoint := heaviestVerifiedCandidate.ForkingPoint.Get()

				return forkingPoint != nil && latestProducedCommitment != nil && (latestProducedCommitment.ID().Slot()-forkingPoint.ID().Slot()) > chainSwitchingThreshold
			}

			return heaviestVerifiedCandidate.LatestProducedCommitment.OnUpdateOnce(func(_ *Commitment, latestProducedCommitment *Commitment) {
				c.Main.Set(heaviestVerifiedCandidate)
			}, chainSwitchingCondition)
		}),
	)
}

func (c *Chains) trackHeaviestCandidate(candidateVar reactive.Variable[*Chain], weightVar func(*Chain) reactive.Variable[uint64], candidate *Chain) (unsubscribe func()) {
	return weightVar(candidate).OnUpdate(func(_ uint64, newWeight uint64) {
		// if the weight of the candidate is higher than the current main chain
		if mainChain := c.Main.Get(); mainChain == nil || newWeight > mainChain.VerifiedWeight.Get() {
			// try to set the candidate as the new heaviest chain
			candidateVar.Compute(func(currentCandidate *Chain) *Chain {
				// only set the candidate as the heaviest chain if it is still the heaviest chain (double-checked locking)
				return lo.Cond(currentCandidate == nil || currentCandidate.IsEvicted.WasTriggered() || newWeight > weightVar(currentCandidate).Get(), candidate, currentCandidate)
			})
		}
	}, true)
}

func (c *Chains) forkChain(forkingPoint *Commitment) *Chain {
	chainLogger, shutdownChainLogger := c.NewEntityLogger("")

	chain := newChain(chainLogger, shutdownChainLogger, c.protocol.LatestSeenSlot)
	chain.ForkingPoint.Set(forkingPoint)

	c.Add(chain)

	return chain
}
