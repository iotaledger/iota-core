package utils

import (
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
)

func increaseOffsets(amount int64, offsets ...*int64) {
	for _, offset := range offsets {
		*offset += amount
	}
}

func WriteValueFunc(writeSeeker io.WriteSeeker, value any, offsetsToIncrease ...*int64) error {
	length := binary.Size(value)
	if length == -1 {
		return ierrors.New("unable to determine length of value")
	}

	if err := binary.Write(writeSeeker, binary.LittleEndian, value); err != nil {
		return ierrors.Wrap(err, "unable to write value")
	}

	increaseOffsets(int64(length), offsetsToIncrease...)

	return nil
}

func WriteBytesFunc(writeSeeker io.WriteSeeker, bytes []byte, offsetsToIncrease ...*int64) error {
	length, err := writeSeeker.Write(bytes)
	if err != nil {
		return ierrors.Wrap(err, "unable to write bytes")
	}

	increaseOffsets(int64(length), offsetsToIncrease...)

	return nil
}
