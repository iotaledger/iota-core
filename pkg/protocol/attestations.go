package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// Attestations is a subcomponent of the protocol that is responsible for handling attestation requests and responses.
type Attestations struct {
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

// newAttestations creates a new attestation protocol instance for the given protocol.
func newAttestations(protocol *Protocol) *Attestations {
	a := &Attestations{
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

// Get returns the commitment, and its attestations (including the corresponding merkle proof).
func (a *Attestations) Get(commitmentID iotago.CommitmentID) (commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], err error) {
	slotAPI, err := a.protocol.Commitments.targetEngine(commitmentID).CommittedSlot(commitmentID)
	if err != nil {
		return nil, nil, nil, ierrors.Wrapf(err, "failed to load committed slot API")
	}

	return slotAPI.Attestations()
}

// Shutdown shuts down the attestation protocol.
func (a *Attestations) Shutdown() {
	a.ticker.Shutdown()
	a.workerPool.Shutdown().ShutdownComplete.Wait()
}

// setupCommitmentVerifier sets up the commitment verifier for the given chain.
func (a *Attestations) setupCommitmentVerifier(chain *Chain) (shutdown func()) {
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
func (a *Attestations) sendRequest(commitmentID iotago.CommitmentID) {
	a.workerPool.Submit(func() {
		if commitment, err := a.protocol.Commitments.Get(commitmentID, false); err == nil {
			a.protocol.Network.RequestAttestations(commitmentID)

			a.LogDebug("request", "commitment", commitment.LogName())
		} else {
			a.LogError("failed to load commitment", "commitmentID", commitmentID, "err", err)
		}
	})
}

// processResponse processes the given attestation response.
func (a *Attestations) processResponse(commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	a.workerPool.Submit(func() {
		publishedCommitment, _, err := a.protocol.Commitments.publishCommitment(commitment)
		if err != nil {
			a.LogDebug("failed to publish commitment when processing attestations", "commitmentID", commitment.ID(), "peer", from, "error", err)

			return
		}

		if publishedCommitment.AttestedWeight.Compute(func(currentWeight uint64) uint64 {
			if !publishedCommitment.RequestAttestations.Get() {
				a.LogTrace("received attestations for previously attested commitment", "commitment", publishedCommitment.LogName())

				return currentWeight
			}

			chain := publishedCommitment.Chain.Get()
			if chain == nil {
				a.LogDebug("failed to find chain for commitment when processing attestations", "commitment", publishedCommitment.LogName())

				return currentWeight
			}

			commitmentVerifier, exists := a.commitmentVerifiers.Get(chain.ForkingPoint.Get().ID())
			if !exists || commitmentVerifier == nil {
				a.LogDebug("failed to retrieve commitment verifier", "commitment", publishedCommitment.LogName())

				return currentWeight
			}

			_, actualWeight, err := commitmentVerifier.verifyCommitment(publishedCommitment, attestations, merkleProof)
			if err != nil {
				a.LogError("failed to verify commitment", "commitment", publishedCommitment.LogName(), "error", err)

				return currentWeight
			}

			if actualWeight > currentWeight {
				a.LogDebug("received response", "commitment", publishedCommitment.LogName(), "fromPeer", from)
			}

			return actualWeight
		}) > 0 {
			publishedCommitment.IsAttested.Set(true)
		}
	})
}

// processRequest processes the given attestation request.
func (a *Attestations) processRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	loggedWorkerPoolTask(a.workerPool, func() error {
		commitment, attestations, proof, err := a.Get(commitmentID)
		if err != nil {
			return ierrors.Wrap(err, "failed to load attestations")
		}

		return a.protocol.Network.SendAttestations(commitment, attestations, proof, from)
	}, a, "commitmentID", commitmentID, "fromPeer", from)
}