package model

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
)

type ValidatorPerformance struct {
	// works if ValidatorBlocksPerSlot is less than 32 because we use it as bit vector
	SlotActivityVector uint32
	// can be uint8 because max count per slot is maximally ValidatorBlocksPerSlot + 1
	BlocksIssuedCount              uint8
	HighestSupportedVersionAndHash VersionAndHash
}

func NewValidatorPerformance() *ValidatorPerformance {
	return &ValidatorPerformance{
		SlotActivityVector:             0,
		BlocksIssuedCount:              0,
		HighestSupportedVersionAndHash: VersionAndHash{},
	}
}

func ValidatorPerformanceFromBytes(bytes []byte) (*ValidatorPerformance, int, error) {
	byteReader := stream.NewByteReader(bytes)

	v, err := ValidatorPerformanceFromReader(byteReader)
	if err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to parse ValidatorPerformance")
	}

	return v, byteReader.BytesRead(), nil
}

func ValidatorPerformanceFromReader(reader io.ReadSeeker) (*ValidatorPerformance, error) {
	var err error
	v := NewValidatorPerformance()

	if v.SlotActivityVector, err = stream.Read[uint32](reader); err != nil {
		return nil, ierrors.Wrap(err, "failed to read SlotActivityVector")
	}
	if v.BlocksIssuedCount, err = stream.Read[uint8](reader); err != nil {
		return nil, ierrors.Wrap(err, "failed to read BlocksIssuedCount")
	}
	if v.HighestSupportedVersionAndHash, err = stream.ReadObject(reader, VersionAndHashSize, VersionAndHashFromBytes); err != nil {
		return nil, ierrors.Wrap(err, "failed to read HighestSupportedVersionAndHash")
	}

	return v, nil
}

func (p *ValidatorPerformance) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer()

	if err := stream.Write(byteBuffer, p.SlotActivityVector); err != nil {
		return nil, ierrors.Wrap(err, "failed to write SlotActivityVector")
	}
	if err := stream.Write(byteBuffer, p.BlocksIssuedCount); err != nil {
		return nil, ierrors.Wrap(err, "failed to write BlocksIssuedCount")
	}
	if err := stream.WriteObject(byteBuffer, p.HighestSupportedVersionAndHash, VersionAndHash.Bytes); err != nil {
		return nil, ierrors.Wrap(err, "failed to write HighestSupportedVersionAndHash")
	}

	return byteBuffer.Bytes()
}
