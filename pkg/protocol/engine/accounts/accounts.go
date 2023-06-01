package accounts

import (
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BlockIssuanceCredits is the minimal interface for the Accounts component of the IOTA protocol.
type BlockIssuanceCredits interface {
	// BIC returns Block Issuer Credits of a specific account for a specific slot index.
	BIC(id iotago.AccountID, slot iotago.SlotIndex) (account Account, err error)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

type KeyOperation bool

var KeyAdded KeyOperation = true
var KeyRemoved KeyOperation = false

type BicDiffChange struct {
	Change              int64
	PreviousUpdatedTime iotago.SlotIndex
	// PubKeysAddedAndRemoved is a map of public keys that were added and removed from the account.
	PubKeysAddedAndRemoved map[ed25519.PublicKey]KeyOperation
}

// UpdateKeys updates the PubKeysAddedAndRemoved map with the added and removed keys from the updateFromDiff.
func (b *BicDiffChange) UpdateKeys(updateFromDiff prunable.BicDiffChange) {
	// operations are already reversed here, as we rollback
	for _, addedKey := range updateFromDiff.PubKeysAdded {
		b.updateKey(addedKey, KeyRemoved)
	}
	for _, removedKey := range updateFromDiff.PubKeysRemoved {
		b.updateKey(removedKey, KeyAdded)
	}
}

// UpdateKeys updates the added and removed keys of the account. It makes sure that two opposite consecutive operations calncel each other.
func (b *BicDiffChange) updateKey(pubKey ed25519.PublicKey, operation KeyOperation) {
	previousOperation, ok := b.PubKeysAddedAndRemoved[pubKey]
	if !ok {
		b.PubKeysAddedAndRemoved[pubKey] = operation
	} else if previousOperation != operation { // cancel the previous operation
		delete(b.PubKeysAddedAndRemoved, pubKey)
	}
}

type ManaHoldings interface {
	// Mana is the stored and potential value of an account collected on the UTXO layer - used by the Scheduler.
	Mana(id iotago.AccountID) (mana *ManaHoldings, err error)
}

type AccountPublicKeys interface {
	IsPublicKeyAllowed(iotago.AccountID, iotago.SlotIndex, ed25519.PublicKey) bool
}

type Account interface {
	ID() iotago.AccountID
	Credits() *Credits
	IsPublicKeyAllowed(ed25519.PublicKey) bool
	Clone() Account
}

type AccountData struct {
	api iotago.API

	id         iotago.AccountID `serix:"0"`
	credits    *Credits         `serix:"1"`
	pubKeysMap *shrinkingmap.ShrinkingMap[ed25519.PublicKey, types.Empty]
}

func NewAccount(api iotago.API, id iotago.AccountID, credits *Credits, pubKeys ...ed25519.PublicKey) *AccountData {
	pubKeysMap := shrinkingmap.New[ed25519.PublicKey, types.Empty](shrinkingmap.WithShrinkingThresholdCount(10))
	if pubKeys != nil {
		for _, pubKey := range pubKeys {
			_ = pubKeysMap.Set(pubKey, types.Void)
		}
	}

	return &AccountData{
		id:         id,
		credits:    credits,
		pubKeysMap: pubKeysMap,
		api:        api,
	}
}

func (a *AccountData) ID() iotago.AccountID {
	return a.id
}

func (a *AccountData) Credits() *Credits {
	return a.credits
}

func (a *AccountData) IsPublicKeyAllowed(pubKey ed25519.PublicKey) bool {
	return a.pubKeysMap.Has(pubKey)
}

func (a *AccountData) AddPublicKey(pubKeys ...ed25519.PublicKey) {
	for _, pubKey := range pubKeys {
		_ = a.pubKeysMap.Set(pubKey, types.Void)
	}
}

func (a *AccountData) RemovePublicKey(pubKeys ...ed25519.PublicKey) {
	for _, pubKey := range pubKeys {
		_ = a.pubKeysMap.Delete(pubKey)
	}
}

func (a *AccountData) Clone() Account {
	keyMapCopy := shrinkingmap.New[ed25519.PublicKey, types.Empty](shrinkingmap.WithShrinkingThresholdCount(10))
	a.pubKeysMap.ForEachKey(func(key ed25519.PublicKey) bool {
		keyMapCopy.Set(key, types.Void)
		return true
	})

	return &AccountData{
		id: a.ID(),
		credits: &Credits{
			Value:      a.Credits().Value,
			UpdateTime: a.Credits().UpdateTime,
		},
		pubKeysMap: keyMapCopy,
	}
}

func (a *AccountData) FromBytes(bytes []byte) (int, error) {
	return a.api.Decode(bytes, a)
}

func (a AccountData) Bytes() ([]byte, error) {
	b, err := a.api.Encode(a) // TODO do we need to add here any options?
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (a *AccountData) SnapshotBytes() []byte {
	m := marshalutil.New()
	m.WriteInt64(a.Credits().Value)
	m.WriteBytes(a.Credits().UpdateTime.Bytes())
	m.WriteUint64(uint64(a.pubKeysMap.Size()))
	a.pubKeysMap.ForEachKey(func(pubKey ed25519.PublicKey) bool {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
		return true
	})

	return m.Bytes()
}
