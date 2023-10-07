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

func NewAttestationsRequester(protocol *Protocol) *AttestationsProtocol {
	a := &AttestationsProtocol{
		Logger:              lo.Return1(protocol.Logger.NewChildLogger("Attestations")),
		protocol:            protocol,
		workerPool:          protocol.Workers.CreatePool("Attestations"),
		ticker:              eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentVerifiers: shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	a.ticker.Events.Tick.Hook(a.sendRequest)

	protocol.HookConstructed(func() {
		protocol.OnChainCreated(func(chain *Chain) {
			chain.CheckAttestations.OnUpdate(func(_, requestAttestations bool) {
				forkingPoint := chain.ForkingPoint.Get()

				if requestAttestations {
					if commitmentBeforeForkingPoint := forkingPoint.Parent.Get(); commitmentBeforeForkingPoint != nil {
						a.commitmentVerifiers.GetOrCreate(forkingPoint.ID(), func() *CommitmentVerifier {
							return NewCommitmentVerifier(chain.Engine.Get(), commitmentBeforeForkingPoint.Commitment)
						})
					}
				} else {
					a.commitmentVerifiers.Delete(forkingPoint.ID())
				}
			})
		})

		protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
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
		commitment, _, err := a.protocol.PublishCommitment(commitmentModel)
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
			if !exists {
				a.LogDebug("failed to find commitment verifier for commitment %s when processing attestations", "commitment", commitment.LogName())

				return currentWeight
			}

			_, actualWeight, err := commitmentVerifier.verifyCommitment(commitment, attestations, merkleProof)
			if err != nil {
				a.LogError("failed to verify commitment when processing attestations", "commitment", commitment.LogName(), "error", err)

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
		commitment, err := a.protocol.Commitment(commitmentID, false)
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

		engineInstance := commitment.Engine.Get()
		if engineInstance == nil {
			a.LogTrace("request for chain without engine", "chain", chain.LogName(), "fromPeer", from)

			return
		}

		if engineInstance.Storage.Settings().LatestCommitment().Slot() < commitmentID.Slot() {
			a.LogTrace("requested commitment not verified", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		commitmentModel, err := engineInstance.Storage.Commitments().Load(commitmentID.Slot())
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

		attestations, err := engineInstance.Attestations.Get(commitmentID.Slot())
		if err != nil {
			a.LogError("failed to load requested attestations", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		rootsStorage, err := engineInstance.Storage.Roots(commitmentID.Slot())
		if err != nil {
			a.LogError("failed to load roots storage for requested attestations", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		roots, err := rootsStorage.Load(commitmentID)
		if err != nil {
			a.LogError("failed to load roots for requested attestations", "commitment", commitment.LogName(), "fromPeer", from)

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

func (a *AttestationsProtocol) sendRequest(commitmentID iotago.CommitmentID) {
	a.workerPool.Submit(func() {
		if commitment, err := a.protocol.Commitment(commitmentID, false); err == nil {
			a.protocol.Network.RequestAttestations(commitmentID)

			a.LogDebug("sent request", "commitment", commitment.LogName())
		} else {
			a.LogError("failed to load commitment", "commitmentID", commitmentID, "err", err)
		}
	})
}
