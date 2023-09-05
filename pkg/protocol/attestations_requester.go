package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type AttestationsRequester struct {
	protocol            *Protocol
	requester           *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	commitmentVerifiers *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]
}

func NewAttestationsRequester(protocol *Protocol) *AttestationsRequester {
	a := &AttestationsRequester{
		protocol:            protocol,
		requester:           eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentVerifiers: shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	protocol.HookConstructed(func() {
		protocol.OnChainCreated(func(chain *Chain) {
			chain.RequestAttestations().OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					a.commitmentVerifiers.GetOrCreate(chain.Root().ID(), func() *CommitmentVerifier {
						return NewCommitmentVerifier(chain.EngineR().Get(), chain.Root().Parent().Get().CommitmentModel())
					})
				} else {
					a.commitmentVerifiers.Delete(chain.Root().ID())
				}
			})
		})

		protocol.OnCommitmentCreated(func(commitment *Commitment) {
			commitment.requestAttestations.OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					a.requester.StartTicker(commitment.ID())
				} else {
					a.requester.StopTicker(commitment.ID())
				}
			})
		})
	})

	return a
}

func (a *AttestationsRequester) ProcessAttestationsResponse(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source network.PeerID) {
	commitment := a.protocol.ProcessCommitment(commitmentModel)
	if commitment == nil {
		// TODO: log warning that we received attestations for a commitment that we don't know (maybe punish sender)

		return
	}

	chain := commitment.Chain()
	if chain == nil {
		// TODO: log warning that we received attestations for an unsolid commitment (we did not request that - maybe punish sender)

		return
	}

	commitmentVerifier, exists := a.commitmentVerifiers.Get(chain.Root().ID())
	if !exists {
		// TODO: log debug that we received attestations for a commitment that we did not request

		return
	}

	blockIDs, actualCumulativeWeight, err := commitmentVerifier.verifyCommitment(commitmentModel, attestations, merkleProof)
	if err != nil {
		// TODO: do something with the error

		return
	}

	// TODO: publish blockIDs, actualCumulativeWeight to target commitment
	commitment.attested.Set(true)
	fmt.Println(blockIDs, actualCumulativeWeight, source)
}

func (a *AttestationsRequester) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	mainEngine := a.protocol.MainEngine()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		a.protocol.TriggerError(ierrors.Wrapf(err, "failed to load commitment %s", commitmentID))
		return
	}

	if commitment.ID() != commitmentID {
		return
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		a.protocol.TriggerError(ierrors.Wrapf(err, "failed to load attestations for commitment %s", commitmentID))

		return
	}

	rootsStorage := mainEngine.Storage.Roots(commitmentID.Index())
	if rootsStorage == nil {
		a.protocol.TriggerError(ierrors.Errorf("failed to load roots for commitment %s", commitmentID))

		return
	}

	rootsBytes, err := rootsStorage.Get(kvstore.Key{prunable.RootsKey})
	if err != nil {
		a.protocol.TriggerError(ierrors.Wrapf(err, "failed to load roots for commitment %s", commitmentID))

		return
	}

	var roots iotago.Roots
	lo.PanicOnErr(a.protocol.APIForSlot(commitmentID.Index()).Decode(rootsBytes, &roots))

	a.protocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
}
