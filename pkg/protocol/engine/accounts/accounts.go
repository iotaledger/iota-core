package accounts

import (
	"crypto"
	"crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO do we still need this separate interface, together with ledger
// BlockIssuanceCredits is the minimal interface for the Accounts component of the IOTA protocol.
type BlockIssuanceCredits interface {
	// BIC returns Block Issuer Credits of a specific account for a specific slot index.
	BIC(id iotago.AccountID, slot iotago.SlotIndex) (account *Credits, err error)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

type AccountPublicKeys interface {
	IsPublicKeyAllowed(iotago.AccountID, iotago.SlotIndex, ed25519.PublicKey) bool
}

// TODO if pubkeys are not stored within BICManager, then we can remove this interface and use Credits as the bicTree leaf
type Account interface {
	ID() iotago.AccountID
	Credits() *Credits
	// ManaHoldings returns the updated stored and potential value of an account collected on the UTXO layer - used by the Scheduler.
	ManaHoldings() *ManaHoldings
	IsPublicKeyAllowed(ed25519.PublicKey) bool
	Clone() Account
}

type AccountImpl struct {
	api iotago.API

	id           iotago.AccountID `serix:"0"`
	credits      *Credits         `serix:"1"`
	manaHoldings *ManaHoldings
	pubKeysMap   *shrinkingmap.ShrinkingMap[crypto.PublicKey, types.Empty]
}

func NewAccount(api iotago.API, id iotago.AccountID, credits *Credits, pubKeys ...ed25519.PublicKey) *AccountImpl {
	pubKeysMap := shrinkingmap.New[crypto.PublicKey, types.Empty](shrinkingmap.WithShrinkingThresholdCount(10))
	if pubKeys != nil {
		for _, pubKey := range pubKeys {
			_ = pubKeysMap.Set(pubKey, types.Void)
		}
	}

	return &AccountImpl{
		id:         id,
		credits:    credits,
		pubKeysMap: pubKeysMap,
		api:        api,
	}
}

func (a *AccountImpl) ID() iotago.AccountID {
	return a.id
}

func (a *AccountImpl) Credits() *Credits {
	return a.credits
}

func (a *AccountImpl) ManaHoldings() *ManaHoldings {
	return a.manaHoldings
}

func (a *AccountImpl) IsPublicKeyAllowed(pubKey ed25519.PublicKey) bool {
	return a.pubKeysMap.Has(pubKey)
}

func (a *AccountImpl) Clone() Account {
	keyMapCopy := shrinkingmap.New[crypto.PublicKey, types.Empty](shrinkingmap.WithShrinkingThresholdCount(10))
	a.pubKeysMap.ForEachKey(func(key crypto.PublicKey) bool {
		keyMapCopy.Set(key, types.Void)
		return true
	})

	return &AccountImpl{
		id: a.ID(),
		credits: &Credits{
			Value:      a.Credits().Value,
			UpdateTime: a.Credits().UpdateTime,
		},
		pubKeysMap: keyMapCopy,
	}
}

func (a *AccountImpl) FromBytes(bytes []byte) (int, error) {
	return a.api.Decode(bytes, a)
}

func (a AccountImpl) Bytes() ([]byte, error) {
	b, err := a.api.Encode(a) // TODO do we need to add here any options?
	if err != nil {
		return nil, err
	}
	return b, nil
}
