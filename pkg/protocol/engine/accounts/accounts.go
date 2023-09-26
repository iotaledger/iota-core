package accounts

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
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
			UpdateTime: a.Credits.UpdateTime,
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

func (a *AccountData) FromBytes(b []byte) (int, error) {
	return a.readFromReadSeeker(bytes.NewReader(b))
}

func (a *AccountData) FromReader(readSeeker io.ReadSeeker) error {
	return lo.Return2(a.readFromReadSeeker(readSeeker))
}

func (a *AccountData) readFromReadSeeker(reader io.ReadSeeker) (int, error) {
	var bytesConsumed int

	bytesRead, err := io.ReadFull(reader, a.ID[:])
	if err != nil {
		return bytesConsumed, ierrors.Wrap(err, "unable to read accountID")
	}

	bytesConsumed += bytesRead

	a.Credits = &BlockIssuanceCredits{}

	if err := binary.Read(reader, binary.LittleEndian, &a.Credits.Value); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read account balance value for accountID %s", a.ID)
	}
	bytesConsumed += 8

	if err := binary.Read(reader, binary.LittleEndian, &a.Credits.UpdateTime); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read updatedTime for account balance for accountID %s", a.ID)
	}
	bytesConsumed += iotago.SlotIndexLength

	if err := binary.Read(reader, binary.LittleEndian, &a.ExpirySlot); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read expiry slot for accountID %s", a.ID)
	}
	bytesConsumed += iotago.SlotIndexLength

	if err := binary.Read(reader, binary.LittleEndian, &a.OutputID); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read outputID for accountID %s", a.ID)
	}
	bytesConsumed += iotago.OutputIDLength

	var blockIssuerKeyCount uint8
	if err := binary.Read(reader, binary.LittleEndian, &blockIssuerKeyCount); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read blockIssuerKeyCount count for accountID %s", a.ID)
	}
	bytesConsumed++

	a.BlockIssuerKeys = iotago.NewBlockIssuerKeys()
	for i := uint8(0); i < blockIssuerKeyCount; i++ {
		var blockIssuerKeyType iotago.BlockIssuerKeyType
		if err := binary.Read(reader, binary.LittleEndian, &blockIssuerKeyType); err != nil {
			return bytesConsumed, ierrors.Wrapf(err, "unable to read block issuer key type for accountID %s", a.ID)
		}
		bytesConsumed++

		switch blockIssuerKeyType {
		case iotago.BlockIssuerKeyEd25519PublicKey:
			var ed25519PublicKey ed25519.PublicKey
			bytesRead, err = io.ReadFull(reader, ed25519PublicKey[:])
			if err != nil {
				return bytesConsumed, ierrors.Wrapf(err, "unable to read public key index %d for accountID %s", i, a.ID)
			}
			bytesConsumed += bytesRead
			a.BlockIssuerKeys.Add(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519PublicKey))
		default:
			return bytesConsumed, ierrors.Wrapf(err, "unsupported block issuer key type %d for accountID %s at offset %d", blockIssuerKeyType, a.ID, i)
		}
	}

	if err := binary.Read(reader, binary.LittleEndian, &(a.ValidatorStake)); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read validator stake for accountID %s", a.ID)
	}
	bytesConsumed += iotago.BaseTokenSize

	if err := binary.Read(reader, binary.LittleEndian, &(a.DelegationStake)); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read delegation stake for accountID %s", a.ID)
	}
	bytesConsumed += iotago.BaseTokenSize

	if err := binary.Read(reader, binary.LittleEndian, &(a.FixedCost)); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read fixed cost for accountID %s", a.ID)
	}
	bytesConsumed += iotago.ManaSize

	if err := binary.Read(reader, binary.LittleEndian, &(a.StakeEndEpoch)); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read stake end epoch for accountID %s", a.ID)
	}
	bytesConsumed += iotago.EpochIndexLength

	versionAndHashBytes := make([]byte, model.VersionAndHashSize)
	if err := binary.Read(reader, binary.LittleEndian, versionAndHashBytes); err != nil {
		return bytesConsumed, ierrors.Wrapf(err, "unable to read latest supported protocol version for accountID %s", a.ID)
	}

	if a.LatestSupportedProtocolVersionAndHash, _, err = model.VersionAndHashFromBytes(versionAndHashBytes[:]); err != nil {
		return 0, err
	}

	bytesConsumed += len(versionAndHashBytes)

	return bytesConsumed, nil
}

func (a AccountData) Bytes() ([]byte, error) {
	idBytes, err := a.ID.Bytes()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to marshal account id")
	}
	m := marshalutil.New()
	m.WriteBytes(idBytes)
	m.WriteBytes(lo.PanicOnErr(a.Credits.Bytes()))
	m.WriteUint32(uint32(a.ExpirySlot))
	m.WriteBytes(lo.PanicOnErr(a.OutputID.Bytes()))
	m.WriteByte(byte(a.BlockIssuerKeys.Size()))
	for _, key := range a.BlockIssuerKeys {
		m.WriteBytes(key.Bytes())
	}

	m.WriteUint64(uint64(a.ValidatorStake))
	m.WriteUint64(uint64(a.DelegationStake))
	m.WriteUint64(uint64(a.FixedCost))
	m.WriteUint32(uint32(a.StakeEndEpoch))
	m.WriteBytes(lo.PanicOnErr(a.LatestSupportedProtocolVersionAndHash.Bytes()))

	return m.Bytes(), nil
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
