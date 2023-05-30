package utils

import (
	"encoding/binary"
	"fmt"
	"github.com/pkg/errors"
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

type PositionedWriter struct {
	saved  map[string]int64
	writer io.WriteSeeker
}

func NewPositionedWriter(writer io.WriteSeeker) *PositionedWriter {
	p := &PositionedWriter{
		saved:  make(map[string]int64),
		writer: writer,
	}
	return p
}

func (p *PositionedWriter) save(name string, position int64) {
	p.saved[name] = position
}

func (p *PositionedWriter) WriteBytes(name string, bytes []byte) error {
	if err := WriteBytesFunc(p.writer, name, bytes); err != nil {
		return err
	}
	return nil
}

func (p *PositionedWriter) WriteValue(name string, value interface{}, saveAs ...bool) error {
	if len(saveAs) > 0 {
		currPosition, err := p.writer.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		p.save(name, currPosition)
	}
	if err := WriteValueFunc(p.writer, name, value); err != nil {
		return err
	}
	return nil
}

func (p *PositionedWriter) WriteValueAt(name string, value interface{}) error {
	// seek back to the file position of the counters
	position, ok := p.saved[name]
	currPosition, err := p.writer.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if !ok {
		return errors.Errorf("unable to find saved position for %s", name)
	}
	if currPosition >= position {
		return errors.Errorf("cannot write into the future, current write position %d is greater than or equal to the saved position %d", currPosition, position)
	}
	diff := currPosition - position
	if _, err := p.writer.Seek(-diff, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek to LS counter placeholders: %w", err)
	}
	if err := WriteValueFunc(p.writer, name, value); err != nil {
		return err
	}
	// seek back to the file position of the counters
	if _, err := p.writer.Seek(diff, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek to LS counter placeholders: %w", err)
	}
	return nil
}
