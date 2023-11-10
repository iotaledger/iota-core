package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chains struct {
	reactive.Set[*Chain]

	HeaviestClaimed  reactive.Variable[*Chain]
	HeaviestAttested reactive.Variable[*Chain]
	HeaviestVerified reactive.Variable[*Chain]
	Main             reactive.Variable[*Chain]

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

	trackHeaviestChain := func(targetChainVariable reactive.Variable[*Chain], weightVariable func(*Chain) reactive.Variable[uint64], candidate *Chain) (unsubscribe func()) {
		return weightVariable(candidate).OnUpdate(func(_ uint64, newWeight uint64) {
			if heaviestChain := c.HeaviestVerified.Get(); heaviestChain != nil && newWeight < heaviestChain.VerifiedWeight.Get() {
				return
			}

			targetChainVariable.Compute(func(currentCandidate *Chain) *Chain {
				if currentCandidate == nil || currentCandidate.IsEvicted.WasTriggered() || newWeight > weightVariable(currentCandidate).Get() {
					return candidate
				}

				return currentCandidate
			})
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
	chain := NewChain(c)
	chain.ForkingPoint.Set(forkingPoint)

	c.Add(chain)

	return chain
}

func (c *Chains) initMainChain() {
	c.protocol.LogTrace("initializing main chain")

	mainChain := NewChain(c)
	mainChain.VerifyState.Set(true)
	mainChain.Engine.OnUpdate(func(_ *engine.Engine, newEngine *engine.Engine) { c.protocol.Events.Engine.LinkTo(newEngine.Events) })

	c.Main.Set(mainChain)

	c.Add(mainChain)
}

func (c *Chains) initChainSwitching() {
	c.HeaviestClaimed.OnUpdate(func(prevHeaviestChain *Chain, heaviestChain *Chain) {
		if prevHeaviestChain != nil {
			prevHeaviestChain.VerifyAttestations.Set(false)
		}

		if !heaviestChain.VerifyState.Get() {
			heaviestChain.VerifyAttestations.Set(true)
		}
	})

	c.HeaviestAttested.OnUpdate(func(_ *Chain, heaviestAttestedChain *Chain) {
		heaviestAttestedChain.VerifyAttestations.Set(false)
		heaviestAttestedChain.VerifyState.Set(true)
	})

	c.HeaviestVerified.OnUpdate(func(_ *Chain, heaviestVerifiedChain *Chain) {
		heaviestVerifiedChain.LatestVerifiedCommitment.OnUpdate(func(_ *Commitment, latestVerifiedCommitment *Commitment) {
			forkingPoint := heaviestVerifiedChain.ForkingPoint.Get()
			if forkingPoint == nil || latestVerifiedCommitment == nil {
				return
			}

			distanceFromForkingPoint := latestVerifiedCommitment.ID().Slot() - forkingPoint.ID().Slot()
			if distanceFromForkingPoint > c.protocol.Options.ChainSwitchingThreshold {
				c.Main.Set(heaviestVerifiedChain)
			}
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
							// publish the root commitment (very first commitment ever)
							rootCommitment, _ := publishCommitment(engine.RootCommitment.Get())
							rootCommitment.IsRoot.Set(true)
							rootCommitment.setChain(chain)

							chain.ForkingPoint.Set(rootCommitment)
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
