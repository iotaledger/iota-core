package inx

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Server) ReadIsAccountValidator(_ context.Context, accountInfoRequest *inx.AccountInfoRequest) (*inx.BoolResponse, error) {
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

	return inx.WrapBoolResponse(deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(slot).HasAccount(accountID)), nil
}

func (s *Server) ReadIsCandidate(_ context.Context, accountInfoRequest *inx.AccountInfoRequest) (*inx.BoolResponse, error) {
	slot := iotago.SlotIndex(accountInfoRequest.GetAccountSlot())
	accountID, _, err := iotago.AccountIDFromBytes(accountInfoRequest.AccountId)
	if err != nil {
		return nil, ierrors.Wrap(err, "error when parsing account id")
	}

	return inx.WrapBoolResponse(deps.Protocol.MainEngineInstance().SybilProtection.IsCandidateActive(accountID, deps.Protocol.APIForSlot(slot).TimeProvider().EpochFromSlot(slot))), nil
}
