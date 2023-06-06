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

type Account interface {
	ID() iotago.AccountID
	BlockIssuanceCredits() *BlockIssuanceCredits
	OutputID() iotago.OutputID
	IsPublicKeyAllowed(ed25519.PublicKey) bool
	PubKeys() *advancedset.AdvancedSet[ed25519.PublicKey]
	Clone() Account
}

type AccountData struct {
	api iotago.API

	id       iotago.AccountID      `serix:"0"`
	credits  *BlockIssuanceCredits `serix:"1"`
	outputID iotago.OutputID       `serix:"2"`
	pubKeys  *advancedset.AdvancedSet[ed25519.PublicKey]
}

func NewAccountData(api iotago.API, id iotago.AccountID, credits *BlockIssuanceCredits, outputID iotago.OutputID, pubKeys ...ed25519.PublicKey) *AccountData {
	return &AccountData{
		id:       id,
		credits:  credits,
		outputID: outputID,
		pubKeys:  advancedset.New[ed25519.PublicKey](pubKeys...),
		api:      api,
	}
}

func (a *AccountData) ID() iotago.AccountID {
	return a.id
}

func (a *AccountData) BlockIssuanceCredits() *BlockIssuanceCredits {
	return a.credits
}

func (a *AccountData) OutputID() iotago.OutputID {
	return a.outputID
}

func (a *AccountData) PubKeys() *advancedset.AdvancedSet[ed25519.PublicKey] {
	return a.pubKeys
}

func (a *AccountData) IsPublicKeyAllowed(pubKey ed25519.PublicKey) bool {
	return a.pubKeys.Has(pubKey)
}

func (a *AccountData) AddPublicKey(pubKeys ...ed25519.PublicKey) {
	for _, pubKey := range pubKeys {
		a.pubKeys.Add(pubKey)
	}
}

func (a *AccountData) RemovePublicKey(pubKeys ...ed25519.PublicKey) {
	for _, pubKey := range pubKeys {
		_ = a.pubKeys.Delete(pubKey)
	}
}

func (a *AccountData) Clone() Account {
	keyCopy := advancedset.New[ed25519.PublicKey]()
	a.pubKeys.Range(func(key ed25519.PublicKey) {
		keyCopy.Add(key)
	})

	return &AccountData{
		id: a.ID(),
		credits: &BlockIssuanceCredits{
			Value:      a.BlockIssuanceCredits().Value,
			UpdateTime: a.BlockIssuanceCredits().UpdateTime,
		},
		pubKeys: keyCopy,
	}
}

func (a *AccountData) FromBytes(bytes []byte) (int, error) {
	return a.api.Decode(bytes, a)
}

func (a AccountData) Bytes() ([]byte, error) {
	b, err := a.api.Encode(a) // TODO: do we need to add here any options?
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (a *AccountData) SnapshotBytes() ([]byte, error) {
	idBytes, err := a.id.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal account id")
	}
	m := marshalutil.New()
	m.WriteBytes(idBytes)
	m.WriteInt64(a.BlockIssuanceCredits().Value)
	m.WriteBytes(a.BlockIssuanceCredits().UpdateTime.Bytes())
	m.WriteUint64(uint64(a.pubKeys.Size()))
	a.pubKeys.Range(func(pubKey ed25519.PublicKey) {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
	})

	return m.Bytes(), nil
}
