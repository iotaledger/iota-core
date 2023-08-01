package prunable

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	diffChangePrefix byte = iota
	destroyedAccountsPrefix
)

// AccountDiff represent the storable changes for a single account within a slot.
type AccountDiff struct {
	BICChange iotago.BlockIssuanceCredits

	PreviousUpdatedTime iotago.SlotIndex

	NewExpirySlot      iotago.SlotIndex
	PreviousExpirySlot iotago.SlotIndex

	// OutputID to which the Account has been transitioned to.
	NewOutputID iotago.OutputID

	// OutputID from which the Account has been transitioned from.
	PreviousOutputID iotago.OutputID

	PubKeysAdded   []ed25519.PublicKey
	PubKeysRemoved []ed25519.PublicKey

	ValidatorStakeChange  int64
	DelegationStakeChange int64
	StakeEndEpochChange   int64
	FixedCostChange       int64
}

// NewAccountDiff creates a new AccountDiff instance.
func NewAccountDiff() *AccountDiff {
	return &AccountDiff{
		BICChange:             0,
		PreviousUpdatedTime:   0,
		NewExpirySlot:         0,
		PreviousExpirySlot:    0,
		NewOutputID:           iotago.EmptyOutputID,
		PreviousOutputID:      iotago.EmptyOutputID,
		PubKeysAdded:          make([]ed25519.PublicKey, 0),
		PubKeysRemoved:        make([]ed25519.PublicKey, 0),
		ValidatorStakeChange:  0,
		DelegationStakeChange: 0,
		StakeEndEpochChange:   0,
		FixedCostChange:       0,
	}
}

func (d AccountDiff) Bytes() ([]byte, error) {
	m := marshalutil.New()

	m.WriteInt64(int64(d.BICChange))
	m.WriteUint64(uint64(d.PreviousUpdatedTime))
	m.WriteUint64(uint64(d.NewExpirySlot))
	m.WriteUint64(uint64(d.PreviousExpirySlot))
	m.WriteBytes(lo.PanicOnErr(d.NewOutputID.Bytes()))
	m.WriteBytes(lo.PanicOnErr(d.PreviousOutputID.Bytes()))
	m.WriteUint8(uint8(len(d.PubKeysAdded)))
	for _, pubKey := range d.PubKeysAdded {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
	}
	m.WriteUint8(uint8(len(d.PubKeysRemoved)))
	for _, pubKey := range d.PubKeysRemoved {
		m.WriteBytes(lo.PanicOnErr(pubKey.Bytes()))
	}

	m.WriteInt64(d.ValidatorStakeChange)
	m.WriteInt64(d.DelegationStakeChange)
	m.WriteInt64(d.FixedCostChange)
	m.WriteUint64(uint64(d.StakeEndEpochChange))

	return m.Bytes(), nil
}

func (d *AccountDiff) Clone() *AccountDiff {
	return &AccountDiff{
		BICChange:             d.BICChange,
		PreviousUpdatedTime:   d.PreviousUpdatedTime,
		NewExpirySlot:         d.NewExpirySlot,
		PreviousExpirySlot:    d.PreviousExpirySlot,
		NewOutputID:           d.NewOutputID,
		PreviousOutputID:      d.PreviousOutputID,
		PubKeysAdded:          lo.CopySlice(d.PubKeysAdded),
		PubKeysRemoved:        lo.CopySlice(d.PubKeysRemoved),
		ValidatorStakeChange:  d.ValidatorStakeChange,
		DelegationStakeChange: d.DelegationStakeChange,
		FixedCostChange:       d.FixedCostChange,
		StakeEndEpochChange:   d.StakeEndEpochChange,
	}
}

func (d *AccountDiff) FromBytes(b []byte) (int, error) {
	return d.readFromReadSeeker(bytes.NewReader(b))
}

func (d *AccountDiff) FromReader(readSeeker io.ReadSeeker) error {
	return lo.Return2(d.readFromReadSeeker(readSeeker))
}

func (d *AccountDiff) readFromReadSeeker(reader io.ReadSeeker) (offset int, err error) {
	if err = binary.Read(reader, binary.LittleEndian, &d.BICChange); err != nil {
		return offset, ierrors.Wrap(err, "unable to read account BIC balance value in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.PreviousUpdatedTime); err != nil {
		return offset, ierrors.Wrap(err, "unable to read previous updated time in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.NewExpirySlot); err != nil {
		return offset, ierrors.Wrap(err, "unable to read new expiry slot in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.PreviousExpirySlot); err != nil {
		return offset, ierrors.Wrap(err, "unable to read previous expiry slot in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.NewOutputID); err != nil {
		return offset, ierrors.Wrap(err, "unable to read new outputID in the diff")
	}

	if err = binary.Read(reader, binary.LittleEndian, &d.PreviousOutputID); err != nil {
		return offset, ierrors.Wrap(err, "unable to read previous outputID in the diff")
	}

	keysAdded, bytesRead, err := readPubKeys(reader)
	if err != nil {
		return offset, ierrors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	offset += bytesRead

	d.PubKeysAdded = keysAdded

	keysRemoved, bytesRead, err := readPubKeys(reader)
	if err != nil {
		return offset, ierrors.Wrap(err, "unable to read removed pubKeys in the diff")
	}
	offset += bytesRead

	d.PubKeysRemoved = keysRemoved

	if err = binary.Read(reader, binary.LittleEndian, &d.ValidatorStakeChange); err != nil {
		return offset, ierrors.Wrap(err, "unable to read validator stake change in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.DelegationStakeChange); err != nil {
		return offset, ierrors.Wrap(err, "unable to read delegation stake change in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.FixedCostChange); err != nil {
		return offset, ierrors.Wrap(err, "unable to read fixed cost change in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.StakeEndEpochChange); err != nil {
		return offset, ierrors.Wrap(err, "unable to read new stake end epoch in the diff")
	}
	offset += 8

	return offset, nil
}

func readPubKeys(reader io.ReadSeeker) ([]ed25519.PublicKey, int, error) {
	var bytesConsumed int

	var pubKeysLength uint8
	if err := binary.Read(reader, binary.LittleEndian, &pubKeysLength); err != nil {
		return nil, bytesConsumed, ierrors.Wrap(err, "unable to read pubKeys length in the diff")
	}
	bytesConsumed++

	pubKeys := make([]ed25519.PublicKey, 0, pubKeysLength)
	for k := uint8(0); k < pubKeysLength; k++ {
		pubKey, bytesRead, err := readPubKey(reader)
		if err != nil {
			return nil, bytesConsumed, err
		}
		bytesConsumed += bytesRead

		pubKeys = append(pubKeys, pubKey)
	}

	return pubKeys, bytesConsumed, nil
}

func readPubKey(reader io.ReadSeeker) (pubKey ed25519.PublicKey, offset int, err error) {
	if offset, err = io.ReadFull(reader, pubKey[:]); err != nil {
		return ed25519.PublicKey{}, offset, ierrors.Errorf("unable to read public key: %w", err)
	}

	return pubKey, offset, nil
}

// AccountDiffs is the storable unit of Account changes for all account in a slot.
type AccountDiffs struct {
	api               iotago.API
	slot              iotago.SlotIndex
	diffChangeStore   *kvstore.TypedStore[iotago.AccountID, *AccountDiff]
	destroyedAccounts *kvstore.TypedStore[iotago.AccountID, types.Empty] // TODO is there any store for set of keys only?
}

// NewAccountDiffs creates a new AccountDiffs instance.
func NewAccountDiffs(slot iotago.SlotIndex, store kvstore.KVStore, api iotago.API) *AccountDiffs {
	return &AccountDiffs{
		api:  api,
		slot: slot,
		diffChangeStore: kvstore.NewTypedStore[iotago.AccountID, *AccountDiff](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{diffChangePrefix})),
			iotago.Identifier.Bytes,
			iotago.IdentifierFromBytes,
			(*AccountDiff).Bytes,
			func(bytes []byte) (object *AccountDiff, consumed int, err error) {
				diff := new(AccountDiff)
				n, err := diff.FromBytes(bytes)

				return diff, n, err
			}),
		destroyedAccounts: kvstore.NewTypedStore[iotago.AccountID, types.Empty](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{destroyedAccountsPrefix})),
			iotago.Identifier.Bytes,
			iotago.IdentifierFromBytes,
			types.Empty.Bytes,
			func(bytes []byte) (object types.Empty, consumed int, err error) {
				return types.Void, 0, nil
			}),
	}
}

// Store stores the given accountID as a root block.
func (b *AccountDiffs) Store(accountID iotago.AccountID, accountDiff *AccountDiff, destroyed bool) (err error) {
	if destroyed {
		if err := b.destroyedAccounts.Set(accountID, types.Void); err != nil {
			return ierrors.Wrap(err, "failed to set destroyed account")
		}
	}

	return b.diffChangeStore.Set(accountID, accountDiff)
}

// Load loads accountID and commitmentID for the given blockID.
func (b *AccountDiffs) Load(accountID iotago.AccountID) (accountDiff *AccountDiff, destroyed bool, err error) {
	destroyed, err = b.destroyedAccounts.Has(accountID)
	if err != nil {
		return accountDiff, false, ierrors.Wrap(err, "failed to get destroyed account")
	} // load diff for a destroyed account to recreate the state

	accountDiff, err = b.diffChangeStore.Get(accountID)
	if err != nil {
		return accountDiff, false, ierrors.Wrapf(err, "failed to get account diff for account %s", accountID)
	}

	return accountDiff, destroyed, err
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
func (b *AccountDiffs) Stream(consumer func(accountID iotago.AccountID, accountDiff *AccountDiff, destroyed bool) bool) error {
	// We firstly iterate over the destroyed accounts, as they won't have a corresponding accountDiff.
	if storageErr := b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID, nil, true)
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over account diffs for slot %s", b.slot)
	}

	// For those accounts that still exist, we might have an accountDiff.
	if storageErr := b.diffChangeStore.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, accountDiff *AccountDiff) bool {
		return consumer(accountID, accountDiff, false)
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over account diffs for slot %s", b.slot)
	}

	return nil
}

// StreamDestroyed streams all destroyed accountIDs for a slot index.
func (b *AccountDiffs) StreamDestroyed(consumer func(accountID iotago.AccountID) bool) error {
	return b.destroyedAccounts.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, empty types.Empty) bool {
		return consumer(accountID)
	})
}
