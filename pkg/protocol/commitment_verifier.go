package protocol

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type CommitmentVerifier struct {
	engine                   *engine.Engine
	lastCommonSlotBeforeFork iotago.SlotIndex

	// epoch is the epoch of the currently verified commitment. Initially, it is set to the epoch of the last common commitment before the fork.
	epoch iotago.EpochIndex

	// validatorAccountsData is the accounts data of the validators for the current epoch as known at lastCommonSlotBeforeFork.
	// Initially, it is set to the accounts data of the validators for the epoch of the last common commitment before the fork.
	validatorAccountsData map[iotago.AccountID]*accounts.AccountData

	// mutex is used to synchronize access to validatorAccountsData and epoch.
	mutex syncutils.RWMutex
}

func newCommitmentVerifier(mainEngine *engine.Engine, lastCommonCommitmentBeforeFork *model.Commitment) (*CommitmentVerifier, error) {
	apiForSlot := mainEngine.APIForSlot(lastCommonCommitmentBeforeFork.Slot())
	epoch := apiForSlot.TimeProvider().EpochFromSlot(lastCommonCommitmentBeforeFork.Slot())

	committeeAtForkingPoint, exists := mainEngine.SybilProtection.SeatManager().CommitteeInEpoch(epoch)
	if !exists {
		return nil, ierrors.Errorf("committee in epoch %d of last commonCommitment slot %d before fork does not exist", epoch, lastCommonCommitmentBeforeFork.Slot())
	}

	accountsAtForkingPoint, err := committeeAtForkingPoint.Accounts()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get accounts from committee for slot %d", lastCommonCommitmentBeforeFork.Slot())
	}

	validatorAccountsDataAtForkingPoint, err := mainEngine.Ledger.PastAccounts(accountsAtForkingPoint.IDs(), lastCommonCommitmentBeforeFork.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get past accounts for slot %d", lastCommonCommitmentBeforeFork.Slot())
	}

	return &CommitmentVerifier{
		engine:                   mainEngine,
		lastCommonSlotBeforeFork: lastCommonCommitmentBeforeFork.Slot(),
		epoch:                    epoch,
		validatorAccountsData:    validatorAccountsDataAtForkingPoint,
	}, nil
}

func (c *CommitmentVerifier) verifyCommitment(commitment *Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier]) (blockIDsFromAttestations iotago.BlockIDs, cumulativeWeight uint64, err error) {
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

	// 2. Update validatorAccountsData if fork happened across epoch boundaries.
	//    We try to use the latest accounts data (at lastCommonSlotBeforeFork) for the current epoch.
	//    This is necessary because the committee might have rotated at the epoch boundary and different validators might be part of it.
	//    In case anything goes wrong we keep using previously known accounts data (initially set to the accounts data
	//    of the validators for the epoch of the last common commitment before the fork).
	c.mutex.Lock()
	apiForSlot := c.engine.APIForSlot(commitment.Slot())
	commitmentEpoch := apiForSlot.TimeProvider().EpochFromSlot(commitment.Slot())
	if commitmentEpoch > c.epoch {
		c.epoch = commitmentEpoch

		committee, exists := c.engine.SybilProtection.SeatManager().CommitteeInEpoch(commitmentEpoch)
		if exists {
			validatorAccounts, err := committee.Accounts()
			if err == nil {
				validatorAccountsData, err := c.engine.Ledger.PastAccounts(validatorAccounts.IDs(), c.lastCommonSlotBeforeFork)
				if err == nil {
					c.validatorAccountsData = validatorAccountsData
				}
			}
		}
	}
	c.mutex.Unlock()

	// 3. Verify attestations.
	blockIDs, seatCount, err := c.verifyAttestations(attestations)
	if err != nil {
		return nil, 0, ierrors.Wrapf(err, "error validating attestations for commitment %s", commitment.ID())
	}

	if seatCount > commitment.Weight.Get() {
		return nil, 0, ierrors.Errorf("invalid cumulative weight for commitment %s: expected %d, got %d", commitment.ID(), commitment.CumulativeWeight(), seatCount)
	}

	return blockIDs, seatCount, nil
}

func (c *CommitmentVerifier) verifyAttestations(attestations []*iotago.Attestation) (iotago.BlockIDs, uint64, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	visitedIdentities := ds.NewSet[iotago.AccountID]()
	var blockIDs iotago.BlockIDs
	var seatCount uint64

	for _, att := range attestations {
		// 1. Make sure the public key used to sign is valid for the given issuerID.
		//    We ignore attestations that don't have a valid public key for the given issuerID.
		//    1. The attestation might be fake.
		//    2. The issuer might have added a new public key in the meantime, but we don't know about it yet
		//       since we only have the ledger state at the forking point.
		accountData, exists := c.validatorAccountsData[att.Header.IssuerID]

		// We don't know the account data of the issuer. Ignore.
		// This could be due to committee rotation at epoch boundary where a new validator (unknown at forking point)
		// is selected into the committee.
		if !exists {
			continue
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

		attestationBlockID, err := att.BlockID()
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "error calculating blockID from attestation")
		}

		// We need to make sure that the issuer is actually part of the committee for the slot of the attestation (issuance of the block).
		// Note: here we're explicitly not using the slot of the commitment we're verifying, but the slot of the attestation.
		// This is because at the time the attestation was created, the committee might have been different from the one at commitment time (due to rotation at epoch boundary).
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
