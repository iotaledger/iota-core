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
	MainChain                 reactive.Variable[*Chain]
	HeaviestClaimedCandidate  reactive.Variable[*Chain]
	HeaviestAttestedCandidate reactive.Variable[*Chain]
	HeaviestVerifiedCandidate reactive.Variable[*Chain]
	CommitmentCreated         *event.Event1[*Commitment]
	ChainCreated              *event.Event1[*Chain]

	protocol      *Protocol
	commitments   *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]
	engineManager *enginemanager.EngineManager

	reactive.EvictionState[iotago.SlotIndex]
}

func newChains(protocol *Protocol) *Chains {
	c := &Chains{
		protocol:                  protocol,
		EvictionState:             reactive.NewEvictionState[iotago.SlotIndex](),
		MainChain:                 reactive.NewVariable[*Chain]().Init(NewChain()),
		HeaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		HeaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		HeaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
		commitments:               shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
		CommitmentCreated:         event.New1[*Commitment](),
		ChainCreated:              event.New1[*Chain](),
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
	}

	c.ChainCreated.Hook(func(chain *Chain) {
		c.provideEngineIfRequested(chain)
		c.publishEngineCommitments(chain)
	})

	c.ChainCreated.Trigger(c.MainChain.Get())

	protocol.HookConstructed(func() {
		c.initMainChain()
		c.initChainSwitching()

		// TODO: trigger initialized
	})

	return c
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

func (c *Chains) MainEngineInstance() *engine.Engine {
	return c.MainChain.Get().Engine.Get()
}

func (c *Chains) initMainChain() {
	mainChain := c.MainChain.Get()
	mainChain.instantiate.Set(true)
	mainChain.Engine.OnUpdate(func(_, newEngine *engine.Engine) {
		c.protocol.Events.Engine.LinkTo(newEngine.Events)
	})
	mainChain.ForkingPoint.Get().IsRoot.Trigger()
}

func (c *Chains) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	if _, err := c.requestCommitment(commitment.PrevID(), true, lo.Void(commitment.Parent.Set)); err != nil {
		c.protocol.LogDebug(ierrors.Wrapf(err, "failed to request parent commitment %s", commitment.PrevID()))
	}

	slotEvictedEvent.OnTrigger(func() {
		commitment.IsEvicted.Trigger()
	})

	c.CommitmentCreated.Trigger(commitment)

	commitment.SpawnedChain.OnUpdate(func(_, newChain *Chain) {
		if newChain != nil {
			c.ChainCreated.Trigger(newChain)
		}
	})
}

func (c *Chains) initChainSwitching() {
	c.HeaviestClaimedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.RequestAttestations.Set(false)
		}

		newCandidate.RequestAttestations.Set(true)
	})

	c.HeaviestAttestedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.instantiate.Set(false)
		}

		newCandidate.instantiate.Set(true)
	})

	c.ChainCreated.Hook(func(chain *Chain) {
		c.trackHeaviestCandidate(c.HeaviestClaimedCandidate, func(chain *Chain) reactive.Variable[uint64] {
			return chain.ClaimedWeight
		}, chain)

		c.trackHeaviestCandidate(c.HeaviestAttestedCandidate, func(chain *Chain) reactive.Variable[uint64] {
			return chain.AttestedWeight
		}, chain)

		c.trackHeaviestCandidate(c.HeaviestVerifiedCandidate, func(chain *Chain) reactive.Variable[uint64] {
			return chain.VerifiedWeight
		}, chain)
	})
}

func (c *Chains) provideEngineIfRequested(chain *Chain) func() {
	return chain.instantiate.OnUpdate(func(_, instantiate bool) {
		if !instantiate {
			chain.spawnedEngine.Set(nil)

			return
		}

		if currentEngine := chain.Engine.Get(); currentEngine == nil {
			mainEngine, err := c.engineManager.LoadActiveEngine(c.protocol.options.SnapshotPath)
			if err != nil {
				panic(fmt.Sprintf("could not load active engine: %s", err))
			}

			chain.spawnedEngine.Set(mainEngine)

			c.protocol.Network.HookStopped(mainEngine.Shutdown)
		} else {
			fmt.Println("WATT IS HIER LOS? EIN FORK?")
		}
	})
}

func (c *Chains) requestCommitment(commitmentID iotago.CommitmentID, requestFromPeers bool, optSuccessCallbacks ...func(metadata *Commitment)) (commitmentRequest *promise.Promise[*Commitment], err error) {
	slotEvicted := c.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() && c.LastEvictedSlot().Get() != 0 {
		forkingPoint := c.MainChain.Get().ForkingPoint.Get()

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
	chain.Engine.OnUpdateWithContext(func(_, engine *engine.Engine, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		if engine != nil {
			withinContext(func() (unsubscribe func()) {
				var latestPublishedIndex iotago.SlotIndex
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

				var rootPublished bool
				return engine.LatestCommitment.OnUpdate(func(_, latestModelCommitment *model.Commitment) {
					if !rootPublished {
						chain.ForkingPoint.Compute(func(currentValue *Commitment) *Commitment {
							if currentValue != nil {
								return currentValue
							}

							return publishCommitment(engine.RootCommitment.Get())
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
		if newChainWeight <= c.MainChain.Get().VerifiedWeight.Get() {
			return
		}

		candidateVariable.Compute(func(currentCandidate *Chain) *Chain {
			if currentCandidate == nil || currentCandidate.IsEvicted.WasTriggered() || newChainWeight > chainWeightVariable(currentCandidate).Get() {
				return candidate
			}

			return currentCandidate
		})
	})
}
