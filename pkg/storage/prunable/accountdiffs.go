package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	diffChangePrefix byte = iota
	destroyedAccountsPrefix
)

// AccountDiff represent the storable changes for a single account within an slot.
type AccountDiff struct {
	Change              int64
	PreviousUpdatedTime iotago.SlotIndex

	// OutputID to which the Account has been transitioned to.
	NewOutputID iotago.OutputID

	// OutputID from which the Account has been transitioned from.
	PreviousOutputID iotago.OutputID

	PubKeysAdded   []ed25519.PublicKey
	PubKeysRemoved []ed25519.PublicKey
}

// NewAccountDiff creates a new AccountDiff instance.
func NewAccountDiff() *AccountDiff {
	return &AccountDiff{
		Change:              0,
		PreviousUpdatedTime: 0,
		NewOutputID:         iotago.EmptyOutputID,
		PreviousOutputID:    iotago.EmptyOutputID,
		PubKeysAdded:        make([]ed25519.PublicKey, 0),
		PubKeysRemoved:      make([]ed25519.PublicKey, 0),
	}
}

func (b AccountDiff) Bytes() ([]byte, error) {
	m := marshalutil.New()

	m.WriteInt64(b.Change)
	m.WriteUint64(uint64(b.PreviousUpdatedTime))
	m.WriteBytes(lo.PanicOnErr(b.NewOutputID.Bytes()))
	m.WriteBytes(lo.PanicOnErr(b.PreviousOutputID.Bytes()))
	m.WriteUint64(uint64(len(b.PubKeysAdded)))
	for _, pubKey := range b.PubKeysAdded {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
	}
	m.WriteUint64(uint64(len(b.PubKeysRemoved)))
	for _, pubKey := range b.PubKeysRemoved {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
	}

	return m.Bytes(), nil
}

func (b *AccountDiff) FromBytes(bytes []byte) (int, error) {
	m := marshalutil.New(bytes)

	change, err := m.ReadInt64()
	if err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal change")
	}

	b.Change = change

	previousUpdatedTime, err := m.ReadUint64()
	if err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal previous updated time")
	}

	b.PreviousUpdatedTime = iotago.SlotIndex(previousUpdatedTime)

	if _, err = b.NewOutputID.FromBytes(lo.PanicOnErr(m.ReadBytes(iotago.OutputIDLength))); err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal new output id")
	}

	if _, err = b.PreviousOutputID.FromBytes(lo.PanicOnErr(m.ReadBytes(iotago.OutputIDLength))); err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal previous output id")
	}

	addedPubKeysLen := lo.PanicOnErr(m.ReadUint64())
	b.PubKeysAdded = make([]ed25519.PublicKey, addedPubKeysLen)

	for i := uint64(0); i < addedPubKeysLen; i++ {
		pubKey := ed25519.PublicKey{}
		if _, err = pubKey.FromBytes(lo.PanicOnErr(m.ReadBytes(ed25519.PublicKeySize))); err != nil {
			return 0, errors.Wrap(err, "failed to unmarshal public key")
		}

		b.PubKeysAdded[i] = pubKey
	}

	removedPubKeysLen := lo.PanicOnErr(m.ReadUint64())
	b.PubKeysRemoved = make([]ed25519.PublicKey, removedPubKeysLen)

	for i := uint64(0); i < removedPubKeysLen; i++ {
		pubKey := ed25519.PublicKey{}
		if _, err = pubKey.FromBytes(lo.PanicOnErr(m.ReadBytes(ed25519.PublicKeySize))); err != nil {
			return 0, errors.Wrap(err, "failed to unmarshal public key")
		}

		b.PubKeysRemoved[i] = pubKey
	}

	return m.ReadOffset(), nil
}

// AccountDiffs is the storable unit of Account changes for all account in a slot.
type AccountDiffs struct {
	api               iotago.API
	slot              iotago.SlotIndex
	diffChangeStore   *kvstore.TypedStore[iotago.AccountID, AccountDiff, *iotago.AccountID, *AccountDiff]
	destroyedAccounts *kvstore.TypedStore[iotago.AccountID, types.Empty, *iotago.AccountID, *types.Empty] // TODO is there any store for set of keys only?
}

// NewAccountDiffs creates a new AccountDiffs instance.
func NewAccountDiffs(slot iotago.SlotIndex, store kvstore.KVStore, api iotago.API) *AccountDiffs {
	return &AccountDiffs{
		api:               api,
		slot:              slot,
		diffChangeStore:   kvstore.NewTypedStore[iotago.AccountID, AccountDiff](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{diffChangePrefix}))),
		destroyedAccounts: kvstore.NewTypedStore[iotago.AccountID, types.Empty](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{destroyedAccountsPrefix}))),
	}
}

// Store stores the given accountID as a root block.
func (b *AccountDiffs) Store(accountID iotago.AccountID, accountDiff AccountDiff, destroyed bool) (err error) {
	if destroyed {
		if err := b.destroyedAccounts.Set(accountID, types.Void); err != nil {
			return errors.Wrapf(err, "failed to set destroyed account")
		}
	}

	return b.diffChangeStore.Set(accountID, accountDiff)
}

// Load loads accountID and commitmentID for the given blockID.
func (b *AccountDiffs) Load(accountID iotago.AccountID) (accountDiff AccountDiff, destroyed bool, err error) {
	if destroyed, err = b.destroyedAccounts.Has(accountID); err != nil {
		return accountDiff, false, errors.Wrapf(err, "failed to get destroyed account")
	} else if destroyed {
		return accountDiff, true, nil
	}

	accountDiff, err = b.diffChangeStore.Get(accountID)
	if err != nil {
		return accountDiff, false, errors.Wrapf(err, "failed to get Account diff for account %s", accountID.String())
	}

	return accountDiff, false, err
}

// Has returns true if the given accountID is a root block.
func (b *AccountDiffs) Has(accountID iotago.AccountID) (has bool, err error) {
	return b.diffChangeStore.Has(accountID)
}

// Delete deletes the given accountID from the root blocks.
func (b *AccountDiffs) Delete(accountID iotago.AccountID) (err error) {
	return b.diffChangeStore.Delete(accountID)
}

// Stream streams all accountIDs changes for a slot index.
func (b *AccountDiffs) Stream(consumer func(accountID iotago.AccountID, accountDiff AccountDiff, destroyed bool) bool) error {
	// We firstly iterate over the destroyed accounts, as they won't have a corresponding accountDiff.
	if storageErr := b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID, AccountDiff{}, true)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over account diffs for slot %s", b.slot)
	}

	// For those accounts that still exist we might have a accountDiff.
	if storageErr := b.diffChangeStore.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, accountDiff AccountDiff) bool {
		return consumer(accountID, accountDiff, false)
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over account diffs for slot %s", b.slot)
	}

	return nil
}

// StreamDestroyed streams all destroyed accountIDs for a slot index.
func (b *AccountDiffs) StreamDestroyed(consumer func(accountID iotago.AccountID) bool) error {
	return b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID)
	})
}
