package accounts

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

//nolint:revive
type AccountsData []*AccountData

type AccountData struct {
	ID              iotago.AccountID
	Credits         *BlockIssuanceCredits
	ExpirySlot      iotago.SlotIndex
	OutputID        iotago.OutputID
	BlockIssuerKeys iotago.BlockIssuerKeys

	ValidatorStake                        iotago.BaseToken
	DelegationStake                       iotago.BaseToken
	FixedCost                             iotago.Mana
	StakeEndEpoch                         iotago.EpochIndex
	LatestSupportedProtocolVersionAndHash model.VersionAndHash
}

func NewAccountData(id iotago.AccountID, opts ...options.Option[AccountData]) *AccountData {
	return options.Apply(&AccountData{
		ID:                                    id,
		Credits:                               &BlockIssuanceCredits{},
		ExpirySlot:                            0,
		OutputID:                              iotago.EmptyOutputID,
		BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(),
		ValidatorStake:                        0,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         0,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
	}, opts)
}

func (a *AccountData) AddBlockIssuerKeys(blockIssuerKeys ...iotago.BlockIssuerKey) {
	for _, blockIssuerKey := range blockIssuerKeys {
		k := blockIssuerKey
		a.BlockIssuerKeys.Add(k)
	}
}

func (a *AccountData) RemoveBlockIssuerKey(blockIssuerKeys ...iotago.BlockIssuerKey) {
	for _, blockIssuerKey := range blockIssuerKeys {
		a.BlockIssuerKeys.Remove(blockIssuerKey)
	}
}

func (a *AccountData) Clone() *AccountData {
	return &AccountData{
		ID: a.ID,
		Credits: &BlockIssuanceCredits{
			Value:      a.Credits.Value,
			UpdateSlot: a.Credits.UpdateSlot,
		},
		ExpirySlot:      a.ExpirySlot,
		OutputID:        a.OutputID,
		BlockIssuerKeys: a.BlockIssuerKeys.Clone(),

		ValidatorStake:                        a.ValidatorStake,
		DelegationStake:                       a.DelegationStake,
		FixedCost:                             a.FixedCost,
		StakeEndEpoch:                         a.StakeEndEpoch,
		LatestSupportedProtocolVersionAndHash: a.LatestSupportedProtocolVersionAndHash,
	}
}

func AccountDataFromReader(reader io.ReadSeeker) (*AccountData, error) {
	accountID, err := stream.Read[iotago.AccountID](reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read accountID")
	}

	a := NewAccountData(accountID)

	if a.Credits, err = stream.ReadObject(reader, BlockIssuanceCreditsBytesLength, BlockIssuanceCreditsFromBytes); err != nil {
		return nil, ierrors.Wrap(err, "unable to read credits")
	}
	if a.ExpirySlot, err = stream.Read[iotago.SlotIndex](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read expiry slot")
	}
	if a.OutputID, err = stream.Read[iotago.OutputID](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read outputID")
	}

	if a.BlockIssuerKeys, err = stream.ReadObjectFromReader(reader, iotago.BlockIssuerKeysFromReader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read block issuer keys")
	}

	if a.ValidatorStake, err = stream.Read[iotago.BaseToken](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read validator stake")
	}

	if a.DelegationStake, err = stream.Read[iotago.BaseToken](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read delegation stake")
	}

	if a.FixedCost, err = stream.Read[iotago.Mana](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read fixed cost")
	}

	if a.StakeEndEpoch, err = stream.Read[iotago.EpochIndex](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read stake end epoch")
	}

	if a.LatestSupportedProtocolVersionAndHash, err = stream.ReadObject(reader, model.VersionAndHashSize, model.VersionAndHashFromBytes); err != nil {
		return nil, ierrors.Wrap(err, "unable to read latest supported protocol version and hash")
	}

	return a, nil
}

func AccountDataFromBytes(b []byte) (*AccountData, int, error) {
	reader := stream.NewByteReader(b)

	a, err := AccountDataFromReader(reader)

	return a, reader.BytesRead(), err
}

func (a *AccountData) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer()

	if err := stream.Write(byteBuffer, a.ID); err != nil {
		return nil, ierrors.Wrap(err, "failed to write AccountID")
	}
	if err := stream.WriteObject(byteBuffer, a.Credits, (*BlockIssuanceCredits).Bytes); err != nil {
		return nil, ierrors.Wrap(err, "failed to write Credits")
	}
	if err := stream.Write(byteBuffer, a.ExpirySlot); err != nil {
		return nil, ierrors.Wrap(err, "failed to write ExpirySlot")
	}
	if err := stream.Write(byteBuffer, a.OutputID); err != nil {
		return nil, ierrors.Wrap(err, "failed to write OutputID")
	}

	if err := stream.WriteObject(byteBuffer, a.BlockIssuerKeys, iotago.BlockIssuerKeys.Bytes); err != nil {
		return nil, ierrors.Wrap(err, "failed to write BlockIssuerKeys")
	}

	if err := stream.Write(byteBuffer, a.ValidatorStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write ValidatorStake")
	}
	if err := stream.Write(byteBuffer, a.DelegationStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write DelegationStake")
	}
	if err := stream.Write(byteBuffer, a.FixedCost); err != nil {
		return nil, ierrors.Wrap(err, "failed to write FixedCost")
	}
	if err := stream.Write(byteBuffer, a.StakeEndEpoch); err != nil {
		return nil, ierrors.Wrap(err, "failed to write StakeEndEpoch")
	}
	if err := stream.WriteObject(byteBuffer, a.LatestSupportedProtocolVersionAndHash, model.VersionAndHash.Bytes); err != nil {
		return nil, ierrors.Wrap(err, "failed to write LatestSupportedProtocolVersionAndHash")
	}

	return byteBuffer.Bytes()
}

func WithCredits(credits *BlockIssuanceCredits) options.Option[AccountData] {
	return func(a *AccountData) {
		a.Credits = credits
	}
}

func WithExpirySlot(expirySlot iotago.SlotIndex) options.Option[AccountData] {
	return func(a *AccountData) {
		a.ExpirySlot = expirySlot
	}
}

func WithOutputID(outputID iotago.OutputID) options.Option[AccountData] {
	return func(a *AccountData) {
		a.OutputID = outputID
	}
}

func WithBlockIssuerKeys(blockIssuerKeys ...iotago.BlockIssuerKey) options.Option[AccountData] {
	return func(a *AccountData) {
		for _, blockIssuerKey := range blockIssuerKeys {
			k := blockIssuerKey
			a.BlockIssuerKeys.Add(k)
		}
	}
}

func WithValidatorStake(validatorStake iotago.BaseToken) options.Option[AccountData] {
	return func(a *AccountData) {
		a.ValidatorStake = validatorStake
	}
}

func WithDelegationStake(delegationStake iotago.BaseToken) options.Option[AccountData] {
	return func(a *AccountData) {
		a.DelegationStake = delegationStake
	}
}

func WithFixedCost(fixedCost iotago.Mana) options.Option[AccountData] {
	return func(a *AccountData) {
		a.FixedCost = fixedCost
	}
}

func WithStakeEndEpoch(stakeEndEpoch iotago.EpochIndex) options.Option[AccountData] {
	return func(a *AccountData) {
		a.StakeEndEpoch = stakeEndEpoch
	}
}

func WithLatestSupportedProtocolVersion(versionAndHash model.VersionAndHash) options.Option[AccountData] {
	return func(a *AccountData) {
		a.LatestSupportedProtocolVersionAndHash = versionAndHash
	}
}
