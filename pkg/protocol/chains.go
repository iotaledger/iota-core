package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chains struct {
	protocol *Protocol

	engineManager *enginemanager.EngineManager

	mainChain reactive.Variable[*Chain]

	heaviestClaimedCandidate reactive.Variable[*Chain]

	heaviestAttestedCandidate reactive.Variable[*Chain]

	heaviestVerifiedCandidate reactive.Variable[*Chain]

	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]

	commitmentCreated *event.Event1[*Commitment]

	chainCreated *event.Event1[*Chain]

	reactive.EvictionState[iotago.SlotIndex]
}

func newChains(protocol *Protocol) *Chains {
	c := &Chains{
		protocol: protocol,
		engineManager: enginemanager.New(
			protocol.Workers,
			func(err error) { protocol.LogError(err) },
			protocol.options.BaseDirectory,
			3,
			protocol.options.StorageOptions,
			protocol.options.EngineOptions,
			protocol.options.FilterProvider,
			protocol.options.CommitmentFilterProvider,
			protocol.options.BlockDAGProvider,
			protocol.options.BookerProvider,
			protocol.options.ClockProvider,
			protocol.options.BlockGadgetProvider,
			protocol.options.SlotGadgetProvider,
			protocol.options.SybilProtectionProvider,
			protocol.options.NotarizationProvider,
			protocol.options.AttestationProvider,
			protocol.options.LedgerProvider,
			protocol.options.SchedulerProvider,
			protocol.options.TipManagerProvider,
			protocol.options.TipSelectionProvider,
			protocol.options.RetainerProvider,
			protocol.options.UpgradeOrchestratorProvider,
			protocol.options.SyncManagerProvider,
		),
		EvictionState:             reactive.NewEvictionState[iotago.SlotIndex](),
		mainChain:                 reactive.NewVariable[*Chain](),
		heaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		heaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		heaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
		commitments:               shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
		commitmentCreated:         event.New1[*Commitment](),
		chainCreated:              event.New1[*Chain](),
	}

	c.OnChainCreated(func(chain *Chain) {
		c.provideEngines(chain)
		c.publishEngineCommitments(chain)
	})

	mainChain := NewChain()
	c.mainChain.Set(mainChain)
	c.chainCreated.Trigger(mainChain)

	protocol.HookConstructed(func() {
		mainChain.engine.instantiate.Set(true)

		c.mainChain.Set(mainChain)

		mainChain.forkingPoint.Get().IsRoot.Trigger()

		//protocol.HeaviestVerifiedCandidate().OnUpdate(func(_, newChain *Chain) {
		//	e.mainEngine.Set(newChain.Engine())
		//})

		mainChain.EngineR().OnUpdate(func(_, newEngine *engine.Engine) {
			protocol.Events.Engine.LinkTo(newEngine.Events)
		})

		c.initChainSwitching()

		// TODO: trigger initialized
	})

	return c
}

func (c *Chains) provideEngines(chain *Chain) func() {
	return chain.engine.instantiate.OnUpdate(func(_, instantiate bool) {
		if !instantiate {
			chain.engine.spawnedEngine.Set(nil)

			return
		}

		currentEngine := chain.engine.Get()
		if currentEngine == nil {
			mainEngine, err := c.engineManager.LoadActiveEngine(c.protocol.options.SnapshotPath)
			if err != nil {
				panic(fmt.Sprintf("could not load active engine: %s", err))
			}

			chain.engine.spawnedEngine.Set(mainEngine)

			c.protocol.Network.HookStopped(mainEngine.Shutdown)
		} else {
			fmt.Println("WATT IS HIER LOS? EIN FORK?")
		}
	})
}

func (c *Chains) PublishCommitment(commitment *model.Commitment) (commitmentMetadata *Commitment, err error) {
	request, requestErr := c.requestCommitment(commitment.ID(), false)
	if requestErr != nil {
		return nil, ierrors.Wrapf(requestErr, "failed to request commitment %s", commitment.ID())
	}

	request.Resolve(NewCommitment(commitment)).OnSuccess(func(resolvedMetadata *Commitment) {
		commitmentMetadata = resolvedMetadata
	})

	return commitmentMetadata, nil
}

func (c *Chains) Commitment(commitmentID iotago.CommitmentID, requestMissing ...bool) (commitment *Commitment, err error) {
	commitmentRequest, exists := c.commitments.Get(commitmentID)
	if !exists && lo.First(requestMissing) {
		if commitmentRequest, err = c.requestCommitment(commitmentID, true); err != nil {
			return nil, ierrors.Wrapf(err, "failed to request commitment %s", commitmentID)
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

func (c *Chains) OnCommitmentCreated(callback func(commitment *Commitment)) (unsubscribe func()) {
	return c.commitmentCreated.Hook(callback).Unhook
}

func (c *Chains) OnChainCreated(callback func(chain *Chain)) (unsubscribe func()) {
	return c.chainCreated.Hook(callback).Unhook
}

func (c *Chains) MainChain() *Chain {
	return c.mainChain.Get()
}

func (c *Chains) MainChainR() reactive.Variable[*Chain] {
	return c.mainChain
}

func (c *Chains) MainEngineInstance() *engine.Engine {
	return c.mainChain.Get().Engine()
}

func (c *Chains) MainEngineR() reactive.Variable[*engine.Engine] {
	return c.mainChain.Get().EngineR()
}

func (c *Chains) HeaviestClaimedCandidate() reactive.Variable[*Chain] {
	return c.heaviestClaimedCandidate
}

func (c *Chains) HeaviestAttestedCandidate() reactive.Variable[*Chain] {
	return c.heaviestAttestedCandidate
}

func (c *Chains) HeaviestVerifiedCandidate() reactive.Variable[*Chain] {
	return c.heaviestVerifiedCandidate
}

func (c *Chains) initChainSwitching() {
	c.heaviestClaimedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.requestAttestations.Set(false)
		}

		newCandidate.requestAttestations.Set(true)
	})

	c.heaviestAttestedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.engine.instantiate.Set(false)
		}

		newCandidate.engine.instantiate.Set(true)
	})

	c.OnChainCreated(func(chain *Chain) {
		c.trackHeaviestCandidate(c.heaviestClaimedCandidate, (*Chain).ClaimedWeight, chain)
		c.trackHeaviestCandidate(c.heaviestAttestedCandidate, (*Chain).AttestedWeight, chain)
		c.trackHeaviestCandidate(c.heaviestVerifiedCandidate, (*Chain).VerifiedWeight, chain)
	})
}

func (c *Chains) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	if _, err := c.requestCommitment(commitment.PrevID(), true, lo.Void(commitment.Parent.Set)); err != nil {
		c.protocol.LogDebug(ierrors.Wrapf(err, "failed to request parent commitment %s", commitment.PrevID()))
	}

	slotEvictedEvent.OnTrigger(func() {
		commitment.IsEvicted.Trigger()
	})

	c.commitmentCreated.Trigger(commitment)

	commitment.SpawnedChain.OnUpdate(func(_, newChain *Chain) {
		if newChain != nil {
			c.chainCreated.Trigger(newChain)
		}
	})
}

func (c *Chains) requestCommitment(commitmentID iotago.CommitmentID, requestFromPeers bool, optSuccessCallbacks ...func(metadata *Commitment)) (commitmentRequest *promise.Promise[*Commitment], err error) {
	slotEvicted := c.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() && c.LastEvictedSlot().Get() != 0 {
		forkingPoint := c.mainChain.Get().ForkingPoint()

		if forkingPoint == nil || commitmentID != forkingPoint.ID() {
			return nil, ErrorSlotEvicted
		}

		for _, successCallback := range optSuccessCallbacks {
			successCallback(forkingPoint)
		}

		return promise.New[*Commitment]().Resolve(forkingPoint), nil
	}

	commitmentRequest, requestCreated := c.commitments.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if requestCreated {
		if requestFromPeers {
			c.protocol.commitmentRequester.StartTicker(commitmentID)

			commitmentRequest.OnComplete(func() {
				c.protocol.commitmentRequester.StopTicker(commitmentID)
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

	return commitmentRequest, nil
}

func (c *Chains) publishEngineCommitments(chain *Chain) {
	chain.engine.OnUpdateWithContext(func(_, engine *engine.Engine, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		if engine != nil {
			withinContext(func() (unsubscribe func()) {
				var (
					latestPublishedIndex iotago.SlotIndex
					rootPublished        bool
				)

				publishCommitment := func(commitment *model.Commitment) (publishedCommitment *Commitment) {
					publishedCommitment, err := c.PublishCommitment(commitment)
					if err != nil {
						panic(err) // this can never happen, but we panic to get a stack trace if it ever does
					}

					publishedCommitment.promote(chain)
					publishedCommitment.IsVerified.Trigger()

					latestPublishedIndex = commitment.Index()

					return publishedCommitment
				}

				return engine.LatestCommitment().OnUpdate(func(_, latestModelCommitment *model.Commitment) {
					if !rootPublished {
						publishedRoot := publishCommitment(engine.RootCommitment().Get())

						chain.ForkingPointR().Compute(func(currentValue *Commitment) *Commitment {
							if currentValue != nil {
								return currentValue
							}

							return publishedRoot
						})

						rootPublished = true
					}

					for latestPublishedIndex < latestModelCommitment.Index() {
						if commitmentToPublish, err := engine.Storage.Commitments().Load(latestPublishedIndex + 1); err != nil {
							panic(err) // this should never happen, but we panic to get a stack trace if it does
						} else {
							publishCommitment(commitmentToPublish)
						}
					}
				})
			})
		}
	})
}

func (c *Chains) trackHeaviestCandidate(candidateVariable reactive.Variable[*Chain], chainWeightVariable func(*Chain) reactive.Variable[uint64], candidate *Chain) {
	chainWeightVariable(candidate).OnUpdate(func(_, newChainWeight uint64) {
		if newChainWeight <= c.mainChain.Get().verifiedWeight.Get() {
			return
		}

		candidateVariable.Compute(func(currentCandidate *Chain) *Chain {
			if currentCandidate == nil || currentCandidate.evicted.WasTriggered() || newChainWeight > chainWeightVariable(currentCandidate).Get() {
				return candidate
			}

			return currentCandidate
		})
	})
}
