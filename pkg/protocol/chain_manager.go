package protocol

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainManager struct {
	MainChain             reactive.Variable[*Chain]
	Chains                reactive.Set[*Chain]
	HeaviestChain         reactive.Variable[*Chain]
	HeaviestAttestedChain reactive.Variable[*Chain]
	HeaviestVerifiedChain reactive.Variable[*Chain]
	CommitmentCreated     *event.Event1[*Commitment]

	protocol    *Protocol
	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]

	reactive.EvictionState[iotago.SlotIndex]
}

func newChainManager(protocol *Protocol) *ChainManager {
	c := &ChainManager{
		protocol:              protocol,
		EvictionState:         reactive.NewEvictionState[iotago.SlotIndex](),
		MainChain:             reactive.NewVariable[*Chain](),
		Chains:                reactive.NewSet[*Chain](),
		HeaviestChain:         reactive.NewVariable[*Chain](),
		HeaviestAttestedChain: reactive.NewVariable[*Chain](),
		HeaviestVerifiedChain: reactive.NewVariable[*Chain](),
		commitments:           shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
		CommitmentCreated:     event.New1[*Commitment](),
	}

	c.HeaviestChain.LogUpdates(c.protocol, log.LevelTrace, "Unchecked Heavier Chain", (*Chain).LogName)
	c.HeaviestAttestedChain.LogUpdates(c.protocol, log.LevelTrace, "Attested Heavier Chain", (*Chain).LogName)

	protocol.Constructed.OnTrigger(func() {
		trackHeaviestChain := func(chainVariable reactive.Variable[*Chain], getWeightVariable func(*Chain) reactive.Variable[uint64], candidate *Chain) (unsubscribe func()) {
			return getWeightVariable(candidate).OnUpdate(func(_ uint64, newChainWeight uint64) {
				if heaviestChain := c.HeaviestVerifiedChain.Get(); heaviestChain != nil && newChainWeight < heaviestChain.VerifiedWeight.Get() {
					return
				}

				chainVariable.Compute(func(currentCandidate *Chain) *Chain {
					if currentCandidate == nil || currentCandidate.IsEvicted.WasTriggered() || newChainWeight > getWeightVariable(currentCandidate).Get() {
						return candidate
					}

					return currentCandidate
				})
			}, true)
		}

		c.OnChainCreated(func(chain *Chain) {
			c.publishEngineCommitments(chain)

			trackHeaviestChain(c.HeaviestVerifiedChain, (*Chain).verifiedWeight, chain)
			trackHeaviestChain(c.HeaviestAttestedChain, (*Chain).attestedWeight, chain)
			trackHeaviestChain(c.HeaviestChain, (*Chain).claimedWeight, chain)
		})

		c.initChainSwitching()
	})

	c.initMainChain()

	return c
}

func (c *ChainManager) PublishCommitment(commitment *model.Commitment) (commitmentMetadata *Commitment, published bool, err error) {
	request := c.requestCommitment(commitment.ID(), false)
	if request.WasRejected() {
		return nil, false, ierrors.Wrapf(request.Err(), "failed to request commitment %s", commitment.ID())
	}

	publishedCommitmentMetadata := NewCommitment(commitment, c.protocol)
	request.Resolve(publishedCommitmentMetadata).OnSuccess(func(resolvedMetadata *Commitment) {
		commitmentMetadata = resolvedMetadata
	})

	if published = commitmentMetadata == publishedCommitmentMetadata; published {
		commitmentMetadata.LogDebug("created", "id", commitment.ID())
	}

	return commitmentMetadata, commitmentMetadata == publishedCommitmentMetadata, nil
}

func (c *ChainManager) Commitment(commitmentID iotago.CommitmentID, requestMissing ...bool) (commitment *Commitment, err error) {
	commitmentRequest, exists := c.commitments.Get(commitmentID)
	if !exists && lo.First(requestMissing) {
		if commitmentRequest = c.requestCommitment(commitmentID, true); commitmentRequest.WasRejected() {
			return nil, ierrors.Wrapf(commitmentRequest.Err(), "failed to request commitment %s", commitmentID)
		}
	}

	if commitmentRequest == nil || !commitmentRequest.WasCompleted() {
		return nil, ErrorCommitmentNotFound
	}

	if commitmentRequest.WasRejected() {
		return nil, commitmentRequest.Err()
	}

	return commitmentRequest.Result(), nil
}

func (c *ChainManager) OnChainCreated(callback func(chain *Chain)) (unsubscribe func()) {
	return c.Chains.OnUpdate(func(mutations ds.SetMutations[*Chain]) {
		mutations.AddedElements().Range(callback)
	})
}

func (c *ChainManager) initMainChain() {
	c.protocol.LogDebug("initializing main chain")

	mainChain := NewChain(c.protocol)

	//c.protocol.LogDebug("new chain created", "name", mainChain.LogName(), "forkingPoint", "<snapshot>")

	mainChain.VerifyState.Set(true)
	mainChain.Engine.OnUpdate(func(_, newEngine *engine.Engine) { c.protocol.Events.Engine.LinkTo(newEngine.Events) })

	c.MainChain.Set(mainChain)
	c.Chains.Add(mainChain)
}

func (c *ChainManager) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PreviousCommitmentID(), true, lo.Void(commitment.Parent.Set)).OnError(func(err error) {
		c.protocol.LogDebug("failed to request previous commitment", "prevId", commitment.PreviousCommitmentID(), "error", err)
	})

	slotEvictedEvent.OnTrigger(func() {
		commitment.IsEvicted.Trigger()
	})

	commitment.SpawnedChain.OnUpdate(func(_, newChain *Chain) {
		if newChain != nil {
			c.Chains.Add(newChain)
		}
	})

	c.CommitmentCreated.Trigger(commitment)
}

func (c *ChainManager) initChainSwitching() {
	c.HeaviestChain.OnUpdate(func(prevHeaviestChain, heaviestChain *Chain) {
		if prevHeaviestChain != nil {
			prevHeaviestChain.VerifyAttestations.Set(false)
		}

		if !heaviestChain.VerifyState.Get() {
			heaviestChain.VerifyAttestations.Set(true)
		}
	})

	c.HeaviestAttestedChain.OnUpdate(func(_, heaviestAttestedChain *Chain) {
		heaviestAttestedChain.VerifyAttestations.Set(false)
		heaviestAttestedChain.VerifyState.Set(true)
	})

	c.HeaviestVerifiedChain.OnUpdate(func(_, heaviestVerifiedChain *Chain) {
		heaviestVerifiedChain.LatestVerifiedCommitment.OnUpdate(func(_, latestVerifiedCommitment *Commitment) {
			forkingPoint := heaviestVerifiedChain.ForkingPoint.Get()
			if forkingPoint == nil || latestVerifiedCommitment == nil {
				return
			}

			distanceFromForkingPoint := latestVerifiedCommitment.ID().Slot() - forkingPoint.ID().Slot()
			if distanceFromForkingPoint > c.protocol.Options.ChainSwitchingThreshold {
				c.MainChain.Set(heaviestVerifiedChain)
			}
		})
	})
}

func (c *ChainManager) requestCommitment(commitmentID iotago.CommitmentID, requestFromPeers bool, optSuccessCallbacks ...func(metadata *Commitment)) (commitmentRequest *promise.Promise[*Commitment]) {
	slotEvicted := c.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() && c.LastEvictedSlot().Get() != 0 {
		forkingPoint := c.MainChain.Get().ForkingPoint.Get()

		if forkingPoint == nil || commitmentID != forkingPoint.ID() {
			return promise.New[*Commitment]().Reject(ErrorSlotEvicted)
		}

		for _, successCallback := range optSuccessCallbacks {
			successCallback(forkingPoint)
		}

		return promise.New[*Commitment]().Resolve(forkingPoint)
	}

	commitmentRequest, requestCreated := c.commitments.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if requestCreated {
		if requestFromPeers {
			c.protocol.CommitmentsProtocol.ticker.StartTicker(commitmentID)

			commitmentRequest.OnComplete(func() {
				c.protocol.CommitmentsProtocol.ticker.StopTicker(commitmentID)
			})
		}

		commitmentRequest.OnSuccess(func(commitment *Commitment) {
			c.setupCommitment(commitment, slotEvicted)
		})

		slotEvicted.OnTrigger(func() { c.commitments.Delete(commitmentID) })
	}

	for _, successCallback := range optSuccessCallbacks {
		commitmentRequest.OnSuccess(successCallback)
	}

	return commitmentRequest
}

func (c *ChainManager) publishEngineCommitments(chain *Chain) {
	chain.SpawnedEngine.OnUpdateWithContext(func(_, engine *engine.Engine, unsubscribeOnUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		if engine != nil {
			var latestPublishedSlot iotago.SlotIndex

			publishCommitment := func(commitment *model.Commitment) (publishedCommitment *Commitment, published bool) {
				publishedCommitment, published, err := c.PublishCommitment(commitment)
				if err != nil {
					panic(err) // this can never happen, but we panic to get a stack trace if it ever does
				}

				publishedCommitment.AttestedWeight.Set(publishedCommitment.Weight.Get())
				publishedCommitment.IsAttested.Trigger()
				publishedCommitment.IsVerified.Trigger()

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
							rootCommitment, _ := publishCommitment(engine.RootCommitment.Get())
							rootCommitment.IsRoot.Trigger()
							rootCommitment.setChain(chain)

							chain.ForkingPoint.Set(rootCommitment)
						} else {
							latestPublishedSlot = forkingPoint.Slot() - 1
						}

						return engine.LatestCommitment.OnUpdate(func(_, latestCommitment *model.Commitment) {
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
