package inx

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Server) ReadIsValidatorAccount(_ context.Context, accountInfoRequest *inx.AccountInfoRequest) (*inx.BoolResponse, error) {
	slot := iotago.SlotIndex(accountInfoRequest.GetAccountSlot())
	accountID, _, err := iotago.AccountIDFromBytes(accountInfoRequest.AccountId)
	if err != nil {
		return nil, ierrors.Wrap(err, "error when parsing account id")
	}

	account, exists, err := deps.Protocol.MainEngineInstance().Ledger.Account(accountID, slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error when retrieving account data for %s", accountID)
	}

	return inx.WrapBoolResponse(exists && account.StakeEndEpoch <= deps.Protocol.APIForSlot(slot).TimeProvider().EpochFromSlot(slot)), nil
}

func (s *Server) ReadIsCommitteeMember(_ context.Context, accountInfoRequest *inx.AccountInfoRequest) (*inx.BoolResponse, error) {
	slot := iotago.SlotIndex(accountInfoRequest.GetAccountSlot())
	accountID, _, err := iotago.AccountIDFromBytes(accountInfoRequest.AccountId)
	if err != nil {
		return nil, ierrors.Wrap(err, "error when parsing account id")
	}
	committee, exists := deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(slot)
	if !exists {
		return nil, ierrors.Errorf("committee does not exist for slot %d", slot)
	}

	return inx.WrapBoolResponse(committee.HasAccount(accountID)), nil
}

func (s *Server) ReadIsCandidate(_ context.Context, accountInfoRequest *inx.AccountInfoRequest) (*inx.BoolResponse, error) {
	slot := iotago.SlotIndex(accountInfoRequest.GetAccountSlot())
	accountID, _, err := iotago.AccountIDFromBytes(accountInfoRequest.AccountId)
	if err != nil {
		return nil, ierrors.Wrap(err, "error when parsing account id")
	}

	isCandidateActive, err := deps.Protocol.MainEngineInstance().SybilProtection.IsCandidateActive(accountID, deps.Protocol.APIForSlot(slot).TimeProvider().EpochFromSlot(slot))
	if err != nil {
		return nil, ierrors.Wrap(err, "error when checking if candidate is active")
	}

	return inx.WrapBoolResponse(isCandidateActive), nil
}
