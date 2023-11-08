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
	cumulativeWeight        uint64
	validatorAccountsAtFork map[iotago.AccountID]*accounts.AccountData
}

func NewCommitmentVerifier(mainEngine *engine.Engine, lastCommonCommitmentBeforeFork *model.Commitment) (*CommitmentVerifier, error) {
	committeeAtForkingPoint, exists := mainEngine.SybilProtection.SeatManager().CommitteeInSlot(lastCommonCommitmentBeforeFork.Slot())
	if !exists {
		return nil, ierrors.Errorf("committee in slot %d does not exist", lastCommonCommitmentBeforeFork.Slot())
	}

	accountsAtForkingPoint, err := committeeAtForkingPoint.Accounts()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get accounts from committee for slot %d", lastCommonCommitmentBeforeFork.Slot())
	}

	return &CommitmentVerifier{
		engine:                  mainEngine,
		cumulativeWeight:        lastCommonCommitmentBeforeFork.CumulativeWeight(),
		validatorAccountsAtFork: lo.PanicOnErr(mainEngine.Ledger.PastAccounts(accountsAtForkingPoint.IDs(), lastCommonCommitmentBeforeFork.Slot())),
		// TODO: what happens if the committee rotated after the fork?
	}, nil
}

func (c *CommitmentVerifier) verifyCommitment(commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier]) (blockIDsFromAttestations iotago.BlockIDs, cumulativeWeight uint64, err error) {
	// 1. Verify that the provided attestations are indeed the ones that were included in the commitment.
	tree := ads.NewMap[iotago.Identifier](mapdb.NewMapDB(),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.AccountID.Bytes,
		iotago.AccountIDFromBytes,
		(*iotago.Attestation).Bytes,
		iotago.AttestationFromBytes(c.engine),
	)

	for _, att := range attestations {
		if err := tree.Set(att.Header.IssuerID, att); err != nil {
			return nil, 0, ierrors.Wrapf(err, "failed to set attestation for issuerID %s", att.Header.IssuerID)
		}
	}
	if !iotago.VerifyProof(merkleProof, tree.Root(), commitment.RootsID()) {
		return nil, 0, ierrors.Errorf("invalid merkle proof for attestations for commitment %s", commitment.ID())
	}

	// 2. Verify attestations.
	blockIDs, seatCount, err := c.verifyAttestations(attestations)
	if err != nil {
		return nil, 0, ierrors.Wrapf(err, "error validating attestations for commitment %s", commitment.ID())
	}

	// 3. Verify that calculated cumulative weight from attestations is lower or equal to cumulative weight of commitment.
	//    This is necessary due to public key changes of validators in the window of forking point and the current state of
	//    the other chain (as validators could have added/removed public keys that we don't know about yet).
	//
	//	1. The weight should be equal if all public keys are known and unchanged.
	//
	//	2. A public key is added to an account.
	//     We do not count a seat for the issuer for this slot and the computed CW will be lower than the CW in
	//	   the commitment. This is fine, since this is a rare occasion and a heavier chain will become heavier anyway, eventually.
	//	   It will simply take a bit longer to accumulate enough CW so that the chain-switch rule kicks in.
	//     Note: In an extreme scenario where all validators add and use a new public key, the chain will never become heavier.
	//           This can only be prevented by adding such key changes provably to the commitments so that these changes
	//           can be reconstructed and verified by nodes that do not have the latest ledger state.
	//
	// 3. A public key is removed from an account.
	//    We count the seat for the issuer for this slot even though we shouldn't have. According to the protocol, a valid
	//    chain with such a block can never exist because the block itself (here provided as an attestation) would be invalid.
	//    However, we do not know about this yet since we do not have the latest ledger state.
	//    In a rare and elaborate scenario, an attacker might have acquired such removed (private) keys and could
	//    fabricate attestations and thus a theoretically heavier chain (solely when looking on the chain backed by attestations)
	//    than it actually is. Nodes might consider to switch to this chain, even though it is invalid which will be discovered
	//    before the candidate chain/engine is activated (it will never get heavier than the current chain).
	c.cumulativeWeight += seatCount
	if c.cumulativeWeight > commitment.CumulativeWeight() {
		return nil, 0, ierrors.Errorf("invalid cumulative weight for commitment %s: expected %d, got %d", commitment.ID(), commitment.CumulativeWeight(), c.cumulativeWeight)
	}

	return blockIDs, c.cumulativeWeight, nil
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
		accountData, exists := c.validatorAccountsAtFork[att.Header.IssuerID]

		// We always need to have the accountData for a validator.
		if !exists {
			return nil, 0, ierrors.Errorf("accountData for issuerID %s does not exist", att.Header.IssuerID)
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
		if visitedIdentities.Has(att.Header.IssuerID) {
			return nil, 0, ierrors.Errorf("issuerID %s contained in multiple attestations", att.Header.IssuerID)
		}

		// TODO: this might differ if we have a Accounts with changing weights depending on the Slot/epoch
		attestationBlockID, err := att.BlockID()
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "error calculating blockID from attestation")
		}

		committee, exists := c.engine.SybilProtection.SeatManager().CommitteeInSlot(attestationBlockID.Slot())
		if !exists {
			return nil, 0, ierrors.Errorf("committee for slot %d does not exist", attestationBlockID.Slot())
		}

		if _, seatExists := committee.GetSeat(att.Header.IssuerID); seatExists {
			seatCount++
		}

		visitedIdentities.Add(att.Header.IssuerID)

		blockID, err := att.BlockID()
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "error calculating blockID from attestation")
		}

		blockIDs = append(blockIDs, blockID)
	}

	return blockIDs, seatCount, nil
}
