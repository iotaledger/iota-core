package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
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

	protocol *Protocol
}

func newChains(protocol *Protocol) *Chains {
	c := &Chains{
		Set:              reactive.NewSet[*Chain](),
		Main:             reactive.NewVariable[*Chain](),
		HeaviestClaimed:  reactive.NewVariable[*Chain](),
		HeaviestAttested: reactive.NewVariable[*Chain](),
		HeaviestVerified: reactive.NewVariable[*Chain](),
		protocol:         protocol,
	}

	c.Main.LogUpdates(c.protocol, log.LevelTrace, "Main", (*Chain).LogName)
	c.HeaviestClaimed.LogUpdates(c.protocol, log.LevelTrace, "HeaviestClaimed", (*Chain).LogName)
	c.HeaviestAttested.LogUpdates(c.protocol, log.LevelTrace, "HeaviestAttested", (*Chain).LogName)

	trackHeaviestChain := func(candidateVar reactive.Variable[*Chain], weightVar func(*Chain) reactive.Variable[uint64], candidate *Chain) (unsubscribe func()) {
		return weightVar(candidate).OnUpdate(func(_ uint64, newWeight uint64) {
			// if the weight of the candidate is higher than the current main chain
			if mainChain := c.Main.Get(); mainChain == nil || newWeight > mainChain.VerifiedWeight.Get() {
				// try to set the candidate as the new heaviest chain
				candidateVar.Compute(func(currentCandidate *Chain) *Chain {
					// only set the candidate as the heaviest chain if it is still the heaviest chain (double locking)
					return lo.Cond(currentCandidate == nil || currentCandidate.IsEvicted.WasTriggered() || newWeight > weightVar(currentCandidate).Get(), candidate, currentCandidate)
				})
			}
		}, true)
	}

	c.WithElements(func(chain *Chain) (teardown func()) {
		return lo.Batch(
			c.publishEngineCommitments(chain),

			trackHeaviestChain(c.HeaviestVerified, func(chain *Chain) reactive.Variable[uint64] { return chain.VerifiedWeight }, chain),
			trackHeaviestChain(c.HeaviestAttested, func(chain *Chain) reactive.Variable[uint64] { return chain.AttestedWeight }, chain),
			trackHeaviestChain(c.HeaviestClaimed, func(chain *Chain) reactive.Variable[uint64] { return chain.ClaimedWeight }, chain),
		)
	})

	c.initChainSwitching()
	c.initMainChain()

	return c
}

func (c *Chains) Fork(forkingPoint *Commitment) *Chain {
	chain := newChain(c)
	chain.ForkingPoint.Set(forkingPoint)

	c.Add(chain)

	return chain
}

func (c *Chains) initMainChain() {
	mainChain := newChain(c)
	mainChain.RequestBlocks.Set(true)

	c.Main.Set(mainChain)

	c.Add(mainChain)
}

func (c *Chains) initChainSwitching() {
	// only switch to the heaviest chain if the latest produced commitment is enough slots away from the forking point.
	chainSwitchingCondition := func(heavierChain *Chain, latestProducedCommitment *Commitment) bool {
		forkingPoint := heavierChain.ForkingPoint.Get()

		return forkingPoint != nil && latestProducedCommitment != nil && (latestProducedCommitment.ID().Slot()-forkingPoint.ID().Slot()) > c.protocol.Options.ChainSwitchingThreshold
	}

	c.HeaviestClaimed.WithNonEmptyValue(func(heaviestClaimed *Chain) (teardown func()) {
		return enable(heaviestClaimed.RequestAttestations)
	})

	c.HeaviestAttested.WithNonEmptyValue(func(heaviestAttested *Chain) (teardown func()) {
		return enable(heaviestAttested.RequestBlocks)
	})

	c.HeaviestVerified.WithNonEmptyValue(func(heaviestVerified *Chain) (teardown func()) {
		return heaviestVerified.LatestProducedCommitment.OnUpdateOnce(func(_ *Commitment, latestProducedCommitment *Commitment) {
			c.Main.Set(heaviestVerified)
		}, func(_ *Commitment, latestProducedCommitment *Commitment) bool {
			return chainSwitchingCondition(heaviestVerified, latestProducedCommitment)
		})
	})
}

func (c *Chains) publishEngineCommitments(chain *Chain) (unsubscribe func()) {
	return chain.SpawnedEngine.OnUpdateWithContext(func(_ *engine.Engine, engine *engine.Engine, unsubscribeOnUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		if engine != nil {
			var latestPublishedSlot iotago.SlotIndex

			publishCommitment := func(commitment *model.Commitment) (publishedCommitment *Commitment, published bool) {
				publishedCommitment, published, err := c.protocol.Commitments.Publish(commitment)
				if err != nil {
					panic(err) // this can never happen, but we panic to get a stack trace if it ever does
				}

				publishedCommitment.AttestedWeight.Set(publishedCommitment.Weight.Get())
				publishedCommitment.IsAttested.Set(true)
				publishedCommitment.IsVerified.Set(true)

				latestPublishedSlot = commitment.Slot()

				if publishedCommitment.IsSolid.Get() {
					publishedCommitment.setChain(chain)
				}

				return publishedCommitment, published
			}

			unsubscribeOnUpdate(func() (unsubscribe func()) {
				return engine.Ledger.HookInitialized(func() {
					unsubscribeOnUpdate(func() (unsubscribe func()) {
						if forkingPoint := chain.ForkingPoint.Get(); forkingPoint == nil {
							// assign root commitment to main chain
							rootCommitment := c.protocol.Commitments.Root.Get()
							rootCommitment.setChain(chain)

							chain.ForkingPoint.Set(rootCommitment)

							latestPublishedSlot = rootCommitment.Slot()
						} else {
							latestPublishedSlot = forkingPoint.Slot() - 1
						}

						return engine.LatestCommitment.OnUpdate(func(_ *model.Commitment, latestCommitment *model.Commitment) {
							for latestPublishedSlot < latestCommitment.Slot() {
								commitmentToPublish, err := engine.Storage.Commitments().Load(latestPublishedSlot + 1)
								if err != nil {
									panic(err) // this should never happen, but we panic to get a stack trace if it does
								}

								publishCommitment(commitmentToPublish)
							}
						})
					})
				})
			})
		}
	})
}

func enable(variable reactive.Variable[bool]) (unset func()) {
	variable.Set(true)

	return func() {
		variable.Set(false)
	}
}
