package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type AttestationsRequester struct {
	chainManager        *ChainManager
	requester           *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	commitmentVerifiers *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]
}

func NewAttestationsRequester(chainManager *ChainManager) *AttestationsRequester {
	a := &AttestationsRequester{
		chainManager:        chainManager,
		requester:           eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentVerifiers: shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	chainManager.chainCreated.Hook(func(chain *Chain) {
		chain.RequestAttestations().OnUpdate(func(_, requestAttestations bool) {
			if requestAttestations {
				a.commitmentVerifiers.GetOrCreate(chain.Root().ID(), func() *CommitmentVerifier {
					return NewCommitmentVerifier(chain.Engine().Get(), chain.Root().Parent().Get().CommitmentModel())
				})
			} else {
				a.commitmentVerifiers.Delete(chain.Root().ID())
			}
		})
	})

	chainManager.commitmentCreated.Hook(func(commitment *Commitment) {
		commitment.requestAttestations.OnUpdate(func(_, requestAttestations bool) {
			if requestAttestations {
				a.requester.StartTicker(commitment.ID())
			} else {
				a.requester.StopTicker(commitment.ID())
			}
		})
	})

	return a
}

func (a *AttestationsRequester) ProcessAttestationsResponse(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source network.PeerID) {
	commitment := a.chainManager.ProcessCommitment(commitmentModel)
	if commitment == nil {
		// TODO: log warning that we received attestations for a commitment that we don't know (maybe punish sender)

		return
	}

	chain := commitment.Chain().Get()
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
