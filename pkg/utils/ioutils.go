package utils

import (
	"encoding/binary"
	"fmt"
	"io"
)

func increaseOffsets(amount int64, offsets ...*int64) {
	for _, offset := range offsets {
		*offset += amount
	}
}

func WriteValueFunc(writeSeeker io.WriteSeeker, variableName string, value any, offsetsToIncrease ...*int64) error {
	length := binary.Size(value)
	if length == -1 {
		return fmt.Errorf("unable to determine length of %s", variableName)
	}

	if err := binary.Write(writeSeeker, binary.LittleEndian, value); err != nil {
		return fmt.Errorf("unable to write LS %s: %w", variableName, err)
	}

	increaseOffsets(int64(length), offsetsToIncrease...)

	return nil
}

func WriteBytesFunc(writeSeeker io.WriteSeeker, variableName string, bytes []byte, offsetsToIncrease ...*int64) error {
	length, err := writeSeeker.Write(bytes)
	if err != nil {
		return fmt.Errorf("unable to write LS %s: %w", variableName, err)
	}

	increaseOffsets(int64(length), offsetsToIncrease...)

	return nil
}
