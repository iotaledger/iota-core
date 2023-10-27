package model

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

// AccountDiff represent the storable changes for a single account within a slot.
type AccountDiff struct {
	BICChange iotago.BlockIssuanceCredits

	PreviousUpdatedSlot iotago.SlotIndex

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
		PreviousUpdatedSlot:               0,
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
	byteBuffer := stream.NewByteBuffer()

	if err := stream.Write(byteBuffer, d.BICChange); err != nil {
		return nil, ierrors.Wrap(err, "unable to write BICChange value in the diff")
	}
	if err := stream.Write(byteBuffer, d.PreviousUpdatedSlot); err != nil {
		return nil, ierrors.Wrap(err, "unable to write PreviousUpdatedSlot in the diff")
	}
	if err := stream.Write(byteBuffer, d.NewExpirySlot); err != nil {
		return nil, ierrors.Wrap(err, "unable to write NewExpirySlot in the diff")
	}
	if err := stream.Write(byteBuffer, d.PreviousExpirySlot); err != nil {
		return nil, ierrors.Wrap(err, "unable to write PreviousExpirySlot in the diff")
	}
	if err := stream.Write(byteBuffer, d.NewOutputID); err != nil {
		return nil, ierrors.Wrap(err, "unable to write NewOutputID in the diff")
	}
	if err := stream.Write(byteBuffer, d.PreviousOutputID); err != nil {
		return nil, ierrors.Wrap(err, "unable to write PreviousOutputID in the diff")
	}

	if err := writeBlockIssuerKeys(byteBuffer, d.BlockIssuerKeysAdded); err != nil {
		return nil, err
	}
	if err := writeBlockIssuerKeys(byteBuffer, d.BlockIssuerKeysRemoved); err != nil {
		return nil, err
	}

	if err := stream.Write(byteBuffer, d.ValidatorStakeChange); err != nil {
		return nil, ierrors.Wrap(err, "unable to write ValidatorStakeChange in the diff")
	}
	if err := stream.Write(byteBuffer, d.DelegationStakeChange); err != nil {
		return nil, ierrors.Wrap(err, "unable to write DelegationStakeChange in the diff")
	}
	if err := stream.Write(byteBuffer, d.FixedCostChange); err != nil {
		return nil, ierrors.Wrap(err, "unable to write FixedCostChange in the diff")
	}
	if err := stream.Write(byteBuffer, d.StakeEndEpochChange); err != nil {
		return nil, ierrors.Wrap(err, "unable to write StakeEndEpochChange in the diff")
	}
	if err := stream.WriteFixedSizeObject(byteBuffer, d.NewLatestSupportedVersionAndHash, VersionAndHashSize, VersionAndHash.Bytes); err != nil {
		return nil, ierrors.Wrap(err, "unable to write NewLatestSupportedVersionAndHash in the diff")
	}
	if err := stream.WriteFixedSizeObject(byteBuffer, d.PrevLatestSupportedVersionAndHash, VersionAndHashSize, VersionAndHash.Bytes); err != nil {
		return nil, ierrors.Wrap(err, "unable to write PrevLatestSupportedVersionAndHash in the diff")
	}

	return byteBuffer.Bytes()
}

func (d *AccountDiff) Clone() *AccountDiff {
	return &AccountDiff{
		BICChange:                         d.BICChange,
		PreviousUpdatedSlot:               d.PreviousUpdatedSlot,
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
	// TODO: remove
	return d.readFromReadSeeker(bytes.NewReader(b))
}

func (d *AccountDiff) FromReader(readSeeker io.ReadSeeker) error {
	// TODO: remove
	return lo.Return2(d.readFromReadSeeker(readSeeker))
}

func (d *AccountDiff) readFromReadSeeker(reader io.ReadSeeker) (offset int, err error) {
	// TODO: adjust to new stream API
	if err = binary.Read(reader, binary.LittleEndian, &d.BICChange); err != nil {
		return offset, ierrors.Wrap(err, "unable to read account BIC balance value in the diff")
	}
	offset += 8

	if err = binary.Read(reader, binary.LittleEndian, &d.PreviousUpdatedSlot); err != nil {
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
		return offset, ierrors.Wrap(err, "unable to read removed blockIssuerKeys in the diff")
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

func writeBlockIssuerKeys(byteBuffer *stream.ByteBuffer, blockIssuerKeys iotago.BlockIssuerKeys) error {
	// TODO: improve this

	blockIssuerKeysBytes, err := iotago.CommonSerixAPI().Encode(context.TODO(), blockIssuerKeys)
	if err != nil {
		return ierrors.Wrap(err, "unable to encode blockIssuerKeys in the diff")
	}

	if err := stream.WriteByteSlice(byteBuffer, blockIssuerKeysBytes, serializer.SeriLengthPrefixTypeAsUint64); err != nil {
		return ierrors.Wrap(err, "unable to write blockIssuerKeysBytes in the diff")
	}

	return nil
}

func readBlockIssuerKeys(reader io.ReadSeeker) (iotago.BlockIssuerKeys, int, error) {
	// TODO: improve this
	var bytesConsumed int

	blockIssuerKeysBytes, err := stream.ReadByteSlice(reader, serializer.SeriLengthPrefixTypeAsUint64)
	if err != nil {
		return nil, bytesConsumed, ierrors.Wrap(err, "unable to read blockIssuerKeysBytes in the diff")
	}

	bytesConsumed += serializer.UInt64ByteSize // add the blob size
	bytesConsumed += len(blockIssuerKeysBytes)

	var blockIssuerKeys iotago.BlockIssuerKeys
	if _, err := iotago.CommonSerixAPI().Decode(context.TODO(), blockIssuerKeysBytes, &blockIssuerKeys); err != nil {
		return nil, bytesConsumed, ierrors.Wrap(err, "unable to decode blockIssuerKeys in the diff")
	}

	return blockIssuerKeys, bytesConsumed, nil
}
