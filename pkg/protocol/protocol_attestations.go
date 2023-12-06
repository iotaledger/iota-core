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
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// AttestationsProtocol is a subcomponent of the protocol that is responsible for handling attestation requests and
// responses.
type AttestationsProtocol struct {
	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// workerPool contains the worker pool that is used to process attestation requests and responses asynchronously.
	workerPool *workerpool.WorkerPool

	// ticker contains the ticker that is used to send attestation requests.
	ticker *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	// commitmentVerifiers contains the commitment verifiers that are used to verify received attestations.
	commitmentVerifiers *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]

	// Logger embeds a logger that can be used to log messages emitted by this component.
	log.Logger
}

// newAttestationsProtocol creates a new attestation protocol instance for the given protocol.
func newAttestationsProtocol(protocol *Protocol) *AttestationsProtocol {
	a := &AttestationsProtocol{
		Logger:              lo.Return1(protocol.Logger.NewChildLogger("Attestations")),
		protocol:            protocol,
		workerPool:          protocol.Workers.CreatePool("Attestations"),
		ticker:              eventticker.New[iotago.SlotIndex, iotago.CommitmentID](protocol.Options.AttestationRequesterOptions...),
		commitmentVerifiers: shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	a.ticker.Events.Tick.Hook(a.sendRequest)

	protocol.Constructed.OnTrigger(func() {
		protocol.Chains.WithElements(func(chain *Chain) (shutdown func()) {
			return chain.RequestAttestations.WithNonEmptyValue(func(requestAttestations bool) (shutdown func()) {
				return a.setupCommitmentVerifier(chain)
			})
		})

		protocol.Commitments.WithElements(func(commitment *Commitment) (shutdown func()) {
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

// ProcessResponse processes the given attestation response.
func (a *AttestationsProtocol) ProcessResponse(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	a.workerPool.Submit(func() {
		commitment, _, err := a.protocol.Commitments.publishCommitmentModel(commitmentModel)
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

// ProcessRequest processes the given attestation request.
func (a *AttestationsProtocol) ProcessRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	a.workerPool.Submit(func() {
		if commitment, err := a.protocol.Commitments.Get(commitmentID, false); err == nil {
			a.processRequest(commitment.TargetEngine(), commitmentID, from)
		} else if ierrors.Is(err, ErrorCommitmentNotFound) {
			a.processRequest(a.protocol.Engines.Main.Get(), commitmentID, from)
		} else {
			a.LogError("failed to load requested commitment", "commitmentID", commitmentID, "fromPeer", from, "err", err)
		}
	})
}

func (a *AttestationsProtocol) processRequest(targetEngine *engine.Engine, commitmentID iotago.CommitmentID, from peer.ID) {
	if targetEngine == nil {
		a.LogTrace("request for commitment without engine", "commitmentID", commitmentID, "fromPeer", from)

		return
	}

	if targetEngine.Storage.Settings().LatestCommitment().Slot() < commitmentID.Slot() {
		a.LogTrace("requested commitment not verified", "commitmentID", commitmentID, "fromPeer", from)

		return
	}

	commitmentModel, err := targetEngine.Storage.Commitments().Load(commitmentID.Slot())
	if err != nil {
		if !ierrors.Is(err, kvstore.ErrKeyNotFound) {
			a.LogError("failed to load requested commitment from engine", "commitmentID", commitmentID, "fromPeer", from, "err", err)
		} else {
			a.LogTrace("requested commitment not found in engine", "commitmentID", commitmentID, "fromPeer", from)
		}

		return
	}

	if commitmentModel.ID() != commitmentID {
		a.LogTrace("commitment ID mismatch", "requestedCommitment", commitmentID, "loadedCommitment", commitmentModel.ID(), "fromPeer", from)

		return
	}

	attestations, err := targetEngine.Attestations.Get(commitmentID.Slot())
	if err != nil {
		a.LogDebug("failed to load requested attestations", "commitmentID", commitmentID, "fromPeer", from)

		return
	}

	rootsStorage, err := targetEngine.Storage.Roots(commitmentID.Slot())
	if err != nil {
		a.LogDebug("failed to load roots storage for requested attestations", "commitmentID", commitmentID, "fromPeer", from)

		return
	}

	roots, exists, err := rootsStorage.Load(commitmentID)
	if err != nil {
		a.LogDebug("failed to load roots for requested attestations", "commitmentID", commitmentID, "err", err, "fromPeer", from)

		return
	} else if !exists {
		a.LogDebug("roots not found for requested attestations", "commitmentID", commitmentID, "fromPeer", from)

		return
	}

	if err = a.protocol.Network.SendAttestations(commitmentModel, attestations, roots.AttestationsProof(), from); err != nil {
		a.LogError("failed to send attestations", "commitmentID", commitmentID, "fromPeer", from, "err", err)

		return
	}

	a.LogTrace("processed request", "commitmentID", commitmentID, "fromPeer", from)
}

// Shutdown shuts down the attestation protocol.
func (a *AttestationsProtocol) Shutdown() {
	a.ticker.Shutdown()
	a.workerPool.Shutdown().ShutdownComplete.Wait()
}

// setupCommitmentVerifier sets up the commitment verifier for the given chain.
func (a *AttestationsProtocol) setupCommitmentVerifier(chain *Chain) (shutdown func()) {
	forkingPoint := chain.ForkingPoint.Get()
	if forkingPoint == nil {
		a.LogError("failed to retrieve forking point", "chain", chain.LogName())

		return nil
	}

	if forkingPoint.IsRoot.Get() {
		a.LogTrace("skipping commitment verifier setup for main chain", "chain", chain.LogName())

		return nil
	}

	parentOfForkingPoint := forkingPoint.Parent.Get()
	if parentOfForkingPoint == nil {
		a.LogError("failed to retrieve parent of forking point", "chain", chain.LogName())

		return nil
	}

	a.commitmentVerifiers.GetOrCreate(forkingPoint.ID(), func() (commitmentVerifier *CommitmentVerifier) {
		commitmentVerifier, err := newCommitmentVerifier(forkingPoint.Chain.Get().LatestEngine(), parentOfForkingPoint.Commitment)
		if err != nil {
			a.LogError("failed to create commitment verifier", "chain", chain.LogName(), "error", err)
		}

		return commitmentVerifier
	})

	return func() {
		a.commitmentVerifiers.Delete(forkingPoint.ID())
	}
}

// sendRequest sends an attestation request for the given commitment ID.
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
