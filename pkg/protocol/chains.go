package protocol

import (
	"time"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Chains represents the set of chains that are managed by the protocol.
type Chains struct {
	// Set contains all chains that are managed by the protocol.
	reactive.Set[*Chain]

	Main             reactive.Variable[*Chain]
	HeaviestClaimed  reactive.Variable[*Chain]
	HeaviestAttested reactive.Variable[*Chain]
	HeaviestVerified reactive.Variable[*Chain]

	LatestSlot reactive.Variable[iotago.SlotIndex]

	protocol *Protocol

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

func newChains(protocol *Protocol) *Chains {
	return (&Chains{
		Set:              reactive.NewSet[*Chain](),
		Main:             reactive.NewVariable[*Chain](),
		HeaviestClaimed:  reactive.NewVariable[*Chain](),
		HeaviestAttested: reactive.NewVariable[*Chain](),
		HeaviestVerified: reactive.NewVariable[*Chain](),
		LatestSlot:       reactive.NewVariable[iotago.SlotIndex](),
		protocol:         protocol,
	}).initLogging(protocol).initBehavior(protocol)
}

func (c *Chains) Fork(forkingPoint *Commitment) *Chain {
	chain := newChain(c)
	chain.ForkingPoint.Set(forkingPoint)

	c.Add(chain)

	return chain
}

func (c *Chains) initLogging(protocol *Protocol) (self *Chains) {
	var shutdownLogger func()
	c.Logger, shutdownLogger = protocol.NewChildLogger("Chains")

	teardownLogging := lo.Batch(
		c.Main.LogUpdates(protocol, log.LevelTrace, "Main", (*Chain).LogName),
		c.HeaviestClaimed.LogUpdates(c, log.LevelTrace, "HeaviestClaimed", (*Chain).LogName),
		c.HeaviestAttested.LogUpdates(c, log.LevelTrace, "HeaviestAttested", (*Chain).LogName),
		c.HeaviestVerified.LogUpdates(c, log.LevelTrace, "HeaviestVerified", (*Chain).LogName),
		c.LatestSlot.LogUpdates(c, log.LevelTrace, "LatestSlot"),

		shutdownLogger,
	)

	protocol.Shutdown.OnTrigger(teardownLogging)

	return c
}

func (c *Chains) initBehavior(protocol *Protocol) (self *Chains) {
	trackHeaviestChain := func(candidateVar reactive.Variable[*Chain], weightVar func(*Chain) reactive.Variable[uint64], candidate *Chain) (unsubscribe func()) {
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

	_ = lo.Batch(
		c.HeaviestClaimed.WithNonEmptyValue(func(heaviestClaimed *Chain) (teardown func()) {
			return toggleTrue(heaviestClaimed.RequestAttestations)
		}),

		c.HeaviestAttested.WithNonEmptyValue(func(heaviestAttested *Chain) (teardown func()) {
			return toggleTrue(heaviestAttested.RequestBlocks)
		}),

		c.HeaviestVerified.WithNonEmptyValue(func(heavierChain *Chain) (teardown func()) {
			// only switch to the heaviest chain if the latest produced commitment is enough slots away from the forking point.
			chainSwitchingCondition := func(_ *Commitment, latestProducedCommitment *Commitment) bool {
				forkingPoint := heavierChain.ForkingPoint.Get()

				return forkingPoint != nil && latestProducedCommitment != nil && (latestProducedCommitment.ID().Slot()-forkingPoint.ID().Slot()) > protocol.Options.ChainSwitchingThreshold
			}

			return heavierChain.LatestProducedCommitment.OnUpdateOnce(func(_ *Commitment, latestProducedCommitment *Commitment) {
				c.Main.Set(heavierChain)
			}, chainSwitchingCondition)
		}),

		c.WithElements(func(chain *Chain) (teardown func()) {
			return lo.Batch(
				trackHeaviestChain(c.HeaviestClaimed, func(chain *Chain) reactive.Variable[uint64] { return chain.ClaimedWeight }, chain),
				trackHeaviestChain(c.HeaviestVerified, func(chain *Chain) reactive.Variable[uint64] { return chain.VerifiedWeight }, chain),
				trackHeaviestChain(c.HeaviestAttested, func(chain *Chain) reactive.Variable[uint64] { return chain.AttestedWeight }, chain),
			)
		}),

		protocol.Constructed.WithNonEmptyValue(func(_ bool) (teardown func()) {
			return protocol.Engines.Main.WithNonEmptyValue(func(mainEngine *engine.Engine) (teardown func()) {
				return c.LatestSlot.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestNetworkTime time.Time) iotago.SlotIndex {
					return mainEngine.LatestAPI().TimeProvider().SlotFromTime(latestNetworkTime)
				}, protocol.Clock))
			})
		}),
	)

	mainChain := newChain(c)
	mainChain.RequestBlocks.Set(true)
	c.Main.Set(mainChain)
	c.Add(mainChain)

	return c
}

func toggleTrue(variable reactive.Variable[bool]) (unset func()) {
	variable.Set(true)

	return func() {
		variable.Set(false)
	}
}
