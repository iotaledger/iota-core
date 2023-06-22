package accounts

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountData struct {
	ID       iotago.AccountID
	Credits  *BlockIssuanceCredits
	OutputID iotago.OutputID
	PubKeys  *advancedset.AdvancedSet[ed25519.PublicKey]

	ValidatorStake  uint64
	DelegationStake uint64
	StakeEndEpoch   iotago.EpochIndex
}

func NewAccountData(id iotago.AccountID, opts ...options.Option[AccountData]) *AccountData {
	return options.Apply(&AccountData{
		ID:              id,
		Credits:         &BlockIssuanceCredits{},
		OutputID:        iotago.EmptyOutputID,
		PubKeys:         advancedset.New[ed25519.PublicKey](),
		ValidatorStake:  0,
		DelegationStake: 0,
		StakeEndEpoch:   0,
	}, opts)
}

func (a *AccountData) IsPublicKeyAllowed(pubKey ed25519.PublicKey) bool {
	return a.PubKeys.Has(pubKey)
}

func (a *AccountData) AddPublicKeys(pubKeys ...ed25519.PublicKey) {
	for _, pubKey := range pubKeys {
		a.PubKeys.Add(pubKey)
	}
}

func (a *AccountData) RemovePublicKeys(pubKeys ...ed25519.PublicKey) {
	for _, pubKey := range pubKeys {
		_ = a.PubKeys.Delete(pubKey)
	}
}

func (a *AccountData) Clone() *AccountData {
	keyCopy := advancedset.New[ed25519.PublicKey]()
	a.PubKeys.Range(func(key ed25519.PublicKey) {
		keyCopy.Add(key)
	})

	return &AccountData{
		ID: a.ID,
		Credits: &BlockIssuanceCredits{
			Value:      a.Credits.Value,
			UpdateTime: a.Credits.UpdateTime,
		},
		OutputID: a.OutputID,
		PubKeys:  keyCopy,

		ValidatorStake:  a.ValidatorStake,
		DelegationStake: a.DelegationStake,
		StakeEndEpoch:   a.StakeEndEpoch,
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
		return bytesConsumed, errors.Wrap(err, "unable to read Account ID")
	}

	bytesConsumed += bytesRead

	a.Credits = &BlockIssuanceCredits{}

	if err := binary.Read(reader, binary.LittleEndian, &a.Credits.Value); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read Account balance value")
	}
	bytesConsumed += 8

	if err := binary.Read(reader, binary.LittleEndian, &a.Credits.UpdateTime); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read updatedTime for Account balance")
	}
	bytesConsumed += 8

	var outputID iotago.OutputID
	if err := binary.Read(reader, binary.LittleEndian, &a.OutputID); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read updatedTime for Account balance")
	}
	bytesConsumed += len(outputID)

	var pubKeyCount int64
	if err := binary.Read(reader, binary.LittleEndian, &pubKeyCount); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read pubKeyCount count")
	}
	bytesConsumed += 8

	pubKeys := make([]ed25519.PublicKey, pubKeyCount)
	for i := int64(0); i < pubKeyCount; i++ {
		var pubKey ed25519.PublicKey
		bytesRead, err = io.ReadFull(reader, pubKey[:])
		if err != nil {
			return bytesConsumed, errors.Wrap(err, "unable to read public key")
		}
		bytesConsumed += bytesRead

		pubKeys[i] = pubKey
	}
	a.PubKeys = advancedset.New(pubKeys...)

	if err := binary.Read(reader, binary.LittleEndian, &(a.ValidatorStake)); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read updatedTime for Account balance")
	}
	bytesConsumed += 8

	if err := binary.Read(reader, binary.LittleEndian, &(a.DelegationStake)); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read delegationStake")
	}
	bytesConsumed += 8

	if err := binary.Read(reader, binary.LittleEndian, &(a.StakeEndEpoch)); err != nil {
		return bytesConsumed, errors.Wrap(err, "unable to read stake end epoch")
	}
	bytesConsumed += 8

	return bytesConsumed, nil
}

func (a AccountData) Bytes() ([]byte, error) {
	idBytes, err := a.ID.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal account id")
	}
	m := marshalutil.New()
	m.WriteBytes(idBytes)
	m.WriteBytes(lo.PanicOnErr(a.Credits.Bytes()))
	m.WriteBytes(lo.PanicOnErr(a.OutputID.Bytes()))
	m.WriteUint64(uint64(a.PubKeys.Size()))
	a.PubKeys.Range(func(pubKey ed25519.PublicKey) {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
	})

	m.WriteUint64(a.ValidatorStake)
	m.WriteUint64(a.DelegationStake)
	m.WriteUint64(uint64(a.StakeEndEpoch))

	return m.Bytes(), nil
}

func WithCredits(credits *BlockIssuanceCredits) options.Option[AccountData] {
	return func(a *AccountData) {
		a.Credits = credits
	}
}

func WithOutputID(outputID iotago.OutputID) options.Option[AccountData] {
	return func(a *AccountData) {
		a.OutputID = outputID
	}
}

func WithPubKeys(pubKeys ...ed25519.PublicKey) options.Option[AccountData] {
	return func(a *AccountData) {
		for _, pubKey := range pubKeys {
			a.PubKeys.Add(pubKey)
		}
	}
}

func WithValidatorStake(validatorStake uint64) options.Option[AccountData] {
	return func(a *AccountData) {
		a.ValidatorStake = validatorStake
	}
}
func WithDelegationStake(delegationStake uint64) options.Option[AccountData] {
	return func(a *AccountData) {
		a.DelegationStake = delegationStake
	}
}

func WithStakeEndEpoch(stakeEndEpoch iotago.EpochIndex) options.Option[AccountData] {
	return func(a *AccountData) {
		a.StakeEndEpoch = stakeEndEpoch
	}
}
