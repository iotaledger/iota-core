package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type AttestationsProtocol struct {
	protocol            *Protocol
	workerPool          *workerpool.WorkerPool
	ticker              *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	commitmentVerifiers *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]

	log.Logger
}

func NewAttestationsProtocol(protocol *Protocol) *AttestationsProtocol {
	a := &AttestationsProtocol{
		Logger:              lo.Return1(protocol.Logger.NewChildLogger("Attestations")),
		protocol:            protocol,
		workerPool:          protocol.Workers.CreatePool("Attestations"),
		ticker:              eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentVerifiers: shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	a.ticker.Events.Tick.Hook(a.sendRequest)

	protocol.Constructed.OnTrigger(func() {
		protocol.Chains.WithElements(func(chain *Chain) (teardown func()) {
			return chain.RequestAttestations.WithNonEmptyValue(func(requestAttestations bool) (teardown func()) {
				return a.setupCommitmentVerifier(chain)
			})
		})

		protocol.Commitments.WithElements(func(commitment *Commitment) (teardown func()) {
			return commitment.RequestAttestations.OnUpdate(func(_ bool, requestAttestations bool) {
				if requestAttestations {
					if commitment.CumulativeWeight() == 0 {
						commitment.IsAttested.Set(true)
					} else {
						a.ticker.StartTicker(commitment.ID())
					}
				} else {
					a.ticker.StopTicker(commitment.ID())
				}
			})
		})
	})

	return a
}

func (a *AttestationsProtocol) ProcessResponse(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	a.workerPool.Submit(func() {
		commitment, _, err := a.protocol.Commitments.Publish(commitmentModel)
		if err != nil {
			a.LogDebug("failed to publish commitment when processing attestations", "commitmentID", commitmentModel.ID(), "peer", from, "error", err)

			return
		}

		if commitment.AttestedWeight.Compute(func(currentWeight uint64) uint64 {
			if !commitment.RequestAttestations.Get() {
				a.LogTrace("received attestations for previously attested commitment", "commitment", commitment.LogName())

				return currentWeight
			}

			chain := commitment.Chain.Get()
			if chain == nil {
				a.LogDebug("failed to find chain for commitment when processing attestations", "commitment", commitment.LogName())

				return currentWeight
			}

			commitmentVerifier, exists := a.commitmentVerifiers.Get(chain.ForkingPoint.Get().ID())
			if !exists || commitmentVerifier == nil {
				a.LogDebug("failed to retrieve commitment verifier", "commitment", commitment.LogName())

				return currentWeight
			}

			_, actualWeight, err := commitmentVerifier.verifyCommitment(commitment, attestations, merkleProof)
			if err != nil {
				a.LogError("failed to verify commitment", "commitment", commitment.LogName(), "error", err)

				return currentWeight
			}

			if actualWeight > currentWeight {
				a.LogDebug("received response", "commitment", commitment.LogName(), "fromPeer", from)
			}

			return actualWeight
		}) > 0 {
			commitment.IsAttested.Set(true)
		}
	})
}

func (a *AttestationsProtocol) ProcessRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	a.workerPool.Submit(func() {
		commitment, err := a.protocol.Commitments.Get(commitmentID, false)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				a.LogError("failed to load requested commitment", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			} else {
				a.LogTrace("failed to load requested commitment", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			}

			return
		}

		chain := commitment.Chain.Get()
		if chain == nil {
			a.LogTrace("request for unsolid commitment", "commitmentID", commitment.LogName(), "fromPeer", from)

			return
		}

		spawnedEngine := commitment.SpawnedEngine()
		if spawnedEngine == nil {
			a.LogTrace("request for chain without engine", "chain", chain.LogName(), "fromPeer", from)

			return
		}

		if spawnedEngine.Storage.Settings().LatestCommitment().Slot() < commitmentID.Slot() {
			a.LogTrace("requested commitment not verified", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		commitmentModel, err := spawnedEngine.Storage.Commitments().Load(commitmentID.Slot())
		if err != nil {
			if !ierrors.Is(err, kvstore.ErrKeyNotFound) {
				a.LogError("failed to load requested commitment from engine", "commitment", commitment.LogName(), "fromPeer", from, "err", err)
			} else {
				a.LogTrace("requested commitment not found in engine", "commitment", commitment.LogName(), "fromPeer", from)
			}

			return
		}

		if commitmentModel.ID() != commitmentID {
			a.LogTrace("commitment ID mismatch", "requestedCommitment", commitment.LogName(), "loadedCommitment", commitmentModel.ID(), "fromPeer", from)

			return
		}

		attestations, err := spawnedEngine.Attestations.Get(commitmentID.Slot())
		if err != nil {
			a.LogError("failed to load requested attestations", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		rootsStorage, err := spawnedEngine.Storage.Roots(commitmentID.Slot())
		if err != nil {
			a.LogError("failed to load roots storage for requested attestations", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		roots, exists, err := rootsStorage.Load(commitmentID)
		if err != nil {
			a.LogError("failed to load roots for requested attestations", "commitment", commitment.LogName(), "err", err, "fromPeer", from)

			return
		} else if !exists {
			a.LogTrace("roots not found for requested attestations", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		if err = a.protocol.Network.SendAttestations(commitmentModel, attestations, roots.AttestationsProof(), from); err != nil {
			a.LogError("failed to send attestations", "commitment", commitment.LogName(), "fromPeer", from, "err", err)
		} else {
			a.LogTrace("processed request", "commitment", commitment.LogName(), "fromPeer", from)
		}
	})
}

func (a *AttestationsProtocol) Shutdown() {
	a.ticker.Shutdown()
	a.workerPool.Shutdown().ShutdownComplete.Wait()
}

func (a *AttestationsProtocol) setupCommitmentVerifier(chain *Chain) (teardown func()) {
	forkingPoint := chain.ForkingPoint.Get()
	if forkingPoint == nil {
		a.LogError("failed to retrieve forking point", "chain", chain.LogName())

		return nil
	}

	parentOfForkingPoint := forkingPoint.Parent.Get()
	if parentOfForkingPoint == nil {
		a.LogError("failed to retrieve parent of forking point", "chain", chain.LogName())

		return nil
	}

	a.commitmentVerifiers.GetOrCreate(forkingPoint.ID(), func() (commitmentVerifier *CommitmentVerifier) {
		commitmentVerifier, err := NewCommitmentVerifier(forkingPoint.Chain.Get().Engine(), parentOfForkingPoint.Commitment)
		if err != nil {
			a.LogError("failed to create commitment verifier", "chain", chain.LogName(), "error", err)
		}

		return commitmentVerifier
	})

	return func() {
		a.commitmentVerifiers.Delete(forkingPoint.ID())
	}
}

func (a *AttestationsProtocol) sendRequest(commitmentID iotago.CommitmentID) {
	a.workerPool.Submit(func() {
		if commitment, err := a.protocol.Commitments.Get(commitmentID, false); err == nil {
			a.protocol.Network.RequestAttestations(commitmentID)

			a.LogDebug("request", "commitment", commitment.LogName())
		} else {
			a.LogError("failed to load commitment", "commitmentID", commitmentID, "err", err)
		}
	})
}
