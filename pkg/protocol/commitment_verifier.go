package protocol

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type CommitmentVerifier struct {
	engine                  *engine.Engine
	validatorAccountsAtFork map[iotago.AccountID]*accounts.AccountData
}

func NewCommitmentVerifier(mainEngine *engine.Engine, lastCommonCommitmentBeforeFork *model.Commitment) *CommitmentVerifier {
	committeeAtForkingPoint := mainEngine.SybilProtection.SeatManager().Committee(lastCommonCommitmentBeforeFork.Slot()).Accounts().IDs()

	return &CommitmentVerifier{
		engine:                  mainEngine,
		validatorAccountsAtFork: lo.PanicOnErr(mainEngine.Ledger.PastAccounts(committeeAtForkingPoint, lastCommonCommitmentBeforeFork.Slot())),
		// TODO: what happens if the committee rotated after the fork?
	}
}

func (c *CommitmentVerifier) verifyCommitment(commitment *Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier]) (blockIDsFromAttestations iotago.BlockIDs, cumulativeWeight uint64, err error) {
	tree := ads.NewMap(mapdb.NewMapDB(), iotago.AccountID.Bytes, iotago.AccountIDFromBytes, (*iotago.Attestation).Bytes, iotago.AttestationFromBytes(c.engine))

	for _, att := range attestations {
		if setErr := tree.Set(att.IssuerID, att); setErr != nil {
			return nil, 0, ierrors.Wrapf(err, "failed to set attestation for issuerID %s", att.IssuerID)
		}
	}

	if !iotago.VerifyProof(merkleProof, iotago.Identifier(tree.Root()), commitment.RootsID()) {
		return nil, 0, ierrors.Errorf("invalid merkle proof for attestations for commitment %s", commitment.ID())
	}

	blockIDs, seatCount, err := c.verifyAttestations(attestations)
	if err != nil {
		return nil, 0, ierrors.Wrapf(err, "error validating attestations for commitment %s", commitment.ID())
	}

	if seatCount > commitment.Weight.Get() {
		return nil, 0, ierrors.Errorf("attestations for commitment %s have more seats (%d) than the commitment weight (%d)", commitment.ID(), seatCount, commitment.Weight.Get())
	}

	return blockIDs, seatCount, nil
}

func (c *CommitmentVerifier) verifyAttestations(attestations []*iotago.Attestation) (iotago.BlockIDs, uint64, error) {
	visitedIdentities := ds.NewSet[iotago.AccountID]()
	var blockIDs iotago.BlockIDs
	var seatCount uint64

	for _, att := range attestations {
		// 1. Make sure the public key used to sign is valid for the given issuerID.
		//    We ignore attestations that don't have a valid public key for the given issuerID.
		//    1. The attestation might be fake.
		//    2. The issuer might have added a new public key in the meantime, but we don't know about it yet
		//       since we only have the ledger state at the forking point.
		accountData, exists := c.validatorAccountsAtFork[att.IssuerID]

		// We always need to have the accountData for a validator.
		if !exists {
			return nil, 0, ierrors.Errorf("accountData for issuerID %s does not exist", att.IssuerID)
		}

		switch signature := att.Signature.(type) {
		case *iotago.Ed25519Signature:
			// We found the accountData, but we don't know the public key used to sign this block/attestation. Ignore.
			if !accountData.BlockIssuerKeys.Has(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(signature.PublicKey)) {
				continue
			}

		default:
			return nil, 0, ierrors.Errorf("only ed25519 signatures supported, got %s", att.Signature.Type())
		}

		// 2. Verify the signature of the attestation.
		if valid, err := att.VerifySignature(); !valid {
			if err != nil {
				return nil, 0, ierrors.Wrap(err, "error validating attestation signature")
			}

			return nil, 0, ierrors.New("invalid attestation signature")
		}

		// 3. A valid set of attestations can't contain multiple attestations from the same issuerID.
		if visitedIdentities.Has(att.IssuerID) {
			return nil, 0, ierrors.Errorf("issuerID %s contained in multiple attestations", att.IssuerID)
		}

		// TODO: this might differ if we have a Accounts with changing weights depending on the SlotIndex/epoch
		attestationBlockID, err := att.BlockID()
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "error calculating blockID from attestation")
		}
		if _, seatExists := c.engine.SybilProtection.SeatManager().Committee(attestationBlockID.Slot()).GetSeat(att.IssuerID); seatExists {
			seatCount++
		}

		visitedIdentities.Add(att.IssuerID)

		blockID, err := att.BlockID()
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "error calculating blockID from attestation")
		}

		blockIDs = append(blockIDs, blockID)
	}

	return blockIDs, seatCount, nil
}
