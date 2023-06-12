package accounts

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountPublicKeys interface {
	IsPublicKeyAllowed(iotago.AccountID, iotago.SlotIndex, ed25519.PublicKey) bool
}

type AccountData struct {
	ID       iotago.AccountID
	Credits  *BlockIssuanceCredits
	OutputID iotago.OutputID
	PubKeys  *advancedset.AdvancedSet[ed25519.PublicKey]
}

func NewAccountData(id iotago.AccountID, credits *BlockIssuanceCredits, outputID iotago.OutputID, pubKeys ...ed25519.PublicKey) *AccountData {
	return &AccountData{
		ID:       id,
		Credits:  credits,
		OutputID: outputID,
		PubKeys:  advancedset.New(pubKeys...),
	}
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
		PubKeys: keyCopy,
	}
}

func (a *AccountData) FromBytes(bytes []byte) (int, error) {
	m := marshalutil.New(bytes)

	if _, err := a.ID.FromBytes(lo.PanicOnErr(m.ReadBytes(iotago.IdentifierLength))); err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal account id")
	}

	a.Credits = &BlockIssuanceCredits{}
	if _, err := a.Credits.FromBytes(lo.PanicOnErr(m.ReadBytes(BlockIssuanceCreditsLength))); err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal block issuance credits")
	}

	if _, err := a.OutputID.FromBytes(lo.PanicOnErr(m.ReadBytes(iotago.OutputIDLength))); err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal output id")
	}

	pubKeysCount, err := m.ReadUint64()
	if err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal public keys count")
	}

	a.PubKeys = advancedset.New[ed25519.PublicKey]()
	for i := uint64(0); i < pubKeysCount; i++ {
		pubKey := ed25519.PublicKey{}
		if _, err = pubKey.FromBytes(lo.PanicOnErr(m.ReadBytes(ed25519.PublicKeySize))); err != nil {
			return 0, errors.Wrap(err, "failed to unmarshal public key")
		}

		a.AddPublicKeys(pubKey)
	}

	return m.ReadOffset(), nil
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

	return m.Bytes(), nil
}
