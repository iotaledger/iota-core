package model

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
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

	BlockIssuerKeysAdded   iotago.BlockIssuerKeys
	BlockIssuerKeysRemoved iotago.BlockIssuerKeys

	ValidatorStakeChange              int64
	DelegationStakeChange             int64
	StakeEndEpochChange               int64
	FixedCostChange                   int64
	PrevLatestSupportedVersionAndHash VersionAndHash
	NewLatestSupportedVersionAndHash  VersionAndHash
}

// NewAccountDiff creates a new AccountDiff instance.
func NewAccountDiff() *AccountDiff {
	return &AccountDiff{
		BICChange:                         0,
		PreviousUpdatedTime:               0,
		NewExpirySlot:                     0,
		PreviousExpirySlot:                0,
		NewOutputID:                       iotago.EmptyOutputID,
		PreviousOutputID:                  iotago.EmptyOutputID,
		BlockIssuerKeysAdded:              iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysRemoved:            iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:              0,
		DelegationStakeChange:             0,
		StakeEndEpochChange:               0,
		FixedCostChange:                   0,
		PrevLatestSupportedVersionAndHash: VersionAndHash{},
		NewLatestSupportedVersionAndHash:  VersionAndHash{},
	}
}

func (d AccountDiff) Bytes() ([]byte, error) {
	m := marshalutil.New()

	m.WriteInt64(int64(d.BICChange))
	m.WriteUint32(uint32(d.PreviousUpdatedTime))
	m.WriteUint32(uint32(d.NewExpirySlot))
	m.WriteUint32(uint32(d.PreviousExpirySlot))
	m.WriteBytes(lo.PanicOnErr(d.NewOutputID.Bytes()))
	m.WriteBytes(lo.PanicOnErr(d.PreviousOutputID.Bytes()))
	m.WriteUint8(uint8(len(d.BlockIssuerKeysAdded)))
	for _, blockIssuerKey := range d.BlockIssuerKeysAdded {
		m.WriteBytes(blockIssuerKey.Bytes())
	}
	m.WriteUint8(uint8(len(d.BlockIssuerKeysRemoved)))
	for _, blockIssuerKey := range d.BlockIssuerKeysRemoved {
		m.WriteBytes(blockIssuerKey.Bytes())
	}

	m.WriteInt64(d.ValidatorStakeChange)
	m.WriteInt64(d.DelegationStakeChange)
	m.WriteInt64(d.FixedCostChange)
	m.WriteUint64(uint64(d.StakeEndEpochChange))
	m.WriteBytes(lo.PanicOnErr(d.NewLatestSupportedVersionAndHash.Bytes()))
	m.WriteBytes(lo.PanicOnErr(d.PrevLatestSupportedVersionAndHash.Bytes()))

	return m.Bytes(), nil
}

func (d *AccountDiff) Clone() *AccountDiff {
	return &AccountDiff{
		BICChange:                         d.BICChange,
		PreviousUpdatedTime:               d.PreviousUpdatedTime,
		NewExpirySlot:                     d.NewExpirySlot,
		PreviousExpirySlot:                d.PreviousExpirySlot,
		NewOutputID:                       d.NewOutputID,
		PreviousOutputID:                  d.PreviousOutputID,
		BlockIssuerKeysAdded:              lo.CopySlice(d.BlockIssuerKeysAdded),
		BlockIssuerKeysRemoved:            lo.CopySlice(d.BlockIssuerKeysRemoved),
		ValidatorStakeChange:              d.ValidatorStakeChange,
		DelegationStakeChange:             d.DelegationStakeChange,
		FixedCostChange:                   d.FixedCostChange,
		StakeEndEpochChange:               d.StakeEndEpochChange,
		NewLatestSupportedVersionAndHash:  d.NewLatestSupportedVersionAndHash,
		PrevLatestSupportedVersionAndHash: d.PrevLatestSupportedVersionAndHash,
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
	offset += iotago.SlotIndexLength

	if err = binary.Read(reader, binary.LittleEndian, &d.NewExpirySlot); err != nil {
		return offset, ierrors.Wrap(err, "unable to read new expiry slot in the diff")
	}
	offset += iotago.SlotIndexLength

	if err = binary.Read(reader, binary.LittleEndian, &d.PreviousExpirySlot); err != nil {
		return offset, ierrors.Wrap(err, "unable to read previous expiry slot in the diff")
	}
	offset += iotago.SlotIndexLength

	if err = binary.Read(reader, binary.LittleEndian, &d.NewOutputID); err != nil {
		return offset, ierrors.Wrap(err, "unable to read new outputID in the diff")
	}
	offset += iotago.OutputIDLength

	if err = binary.Read(reader, binary.LittleEndian, &d.PreviousOutputID); err != nil {
		return offset, ierrors.Wrap(err, "unable to read previous outputID in the diff")
	}
	offset += iotago.OutputIDLength

	keysAdded, bytesRead, err := readBlockIssuerKeys(reader)
	if err != nil {
		return offset, ierrors.Wrap(err, "unable to read added blockIssuerKeys in the diff")
	}
	offset += bytesRead

	d.BlockIssuerKeysAdded = keysAdded

	keysRemoved, bytesRead, err := readBlockIssuerKeys(reader)
	if err != nil {
		return offset, ierrors.Wrap(err, "unable to read removed blockIssuerKey in the diff")
	}
	offset += bytesRead

	d.BlockIssuerKeysRemoved = keysRemoved

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

	newVersionAndHashBytes := make([]byte, VersionAndHashSize)
	if err = binary.Read(reader, binary.LittleEndian, newVersionAndHashBytes); err != nil {
		return offset, ierrors.Wrap(err, "unable to read new version and hash bytes in the diff")
	}
	d.NewLatestSupportedVersionAndHash, _, err = VersionAndHashFromBytes(newVersionAndHashBytes)
	if err != nil {
		return offset, ierrors.Wrap(err, "unable to parse new version and hash bytes in the diff")
	}
	offset += len(newVersionAndHashBytes)

	prevVersionAndHashBytes := make([]byte, VersionAndHashSize)
	if err = binary.Read(reader, binary.LittleEndian, prevVersionAndHashBytes); err != nil {
		return offset, ierrors.Wrap(err, "unable to read prev version and hash bytes in the diff")
	}
	d.PrevLatestSupportedVersionAndHash, _, err = VersionAndHashFromBytes(prevVersionAndHashBytes)
	if err != nil {
		return offset, ierrors.Wrap(err, "unable to parse prev version and hash bytes in the diff")
	}
	offset += len(prevVersionAndHashBytes)

	return offset, nil
}

func readBlockIssuerKeys(reader io.ReadSeeker) (iotago.BlockIssuerKeys, int, error) {
	var bytesConsumed int

	var blockIssuerKeysCount uint8
	if err := binary.Read(reader, binary.LittleEndian, &blockIssuerKeysCount); err != nil {
		return nil, bytesConsumed, ierrors.Wrap(err, "unable to read blockIssuerKeys length in the diff")
	}
	bytesConsumed++

	blockIssuerKeys := iotago.NewBlockIssuerKeys()
	for k := uint8(0); k < blockIssuerKeysCount; k++ {
		blockIssuerKey, bytesRead, err := readBlockIssuerKey(reader)
		if err != nil {
			return nil, bytesConsumed, err
		}
		bytesConsumed += bytesRead

		blockIssuerKeys.Add(blockIssuerKey)
	}

	return blockIssuerKeys, bytesConsumed, nil
}

func readBlockIssuerKey(reader io.ReadSeeker) (iotago.BlockIssuerKey, int, error) {
	bytesConsumed := 0
	var blockIssuerKeyType iotago.BlockIssuerKeyType
	if err := binary.Read(reader, binary.LittleEndian, &blockIssuerKeyType); err != nil {
		return nil, bytesConsumed, ierrors.Wrapf(err, "unable to read block issuer key type in account diff")
	}
	bytesConsumed++

	switch blockIssuerKeyType {
	case iotago.BlockIssuerKeyEd25519PublicKey:
		var ed25519PublicKey ed25519.PublicKey
		var bytesRead, err = io.ReadFull(reader, ed25519PublicKey[:])
		bytesConsumed += bytesRead
		if err != nil {
			return nil, bytesConsumed, ierrors.Errorf("unable to read ed25519 public key in account diff: %w", err)
		}

		return iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519PublicKey), bytesConsumed, nil

	// TODO: we need to add this case
	//case iotago.BlockIssuerKeyEd25519Address:
	//	var ed25519Address *iotago.ImplicitAccountCreationAddress
	//	var bytesRead, err = io.ReadFull(reader, ed25519Address[:])
	//	bytesConsumed += bytesRead
	//	if err != nil {
	//		return nil, bytesConsumed, ierrors.Errorf("unable to read ed25519 address key in account diff: %w", err)
	//	}
	//
	//	return iotago.Ed25519PublicKeyHashBlockIssuerKeyFromAddress(ed25519Address), bytesConsumed, nil

	default:
		return nil, bytesConsumed, ierrors.Errorf("unsupported block issuer key type %d in account diff", blockIssuerKeyType)
	}
}
