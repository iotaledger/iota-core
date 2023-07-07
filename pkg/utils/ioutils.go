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

type PositionedWriter struct {
	bookmarks map[string]int64
	writer    io.WriteSeeker
}

func NewPositionedWriter(writer io.WriteSeeker) *PositionedWriter {
	p := &PositionedWriter{
		bookmarks: make(map[string]int64),
		writer:    writer,
	}

	return p
}

func (p *PositionedWriter) WriteBytes(bytes []byte) error {
	return WriteBytesFunc(p.writer, bytes)
}

func (p *PositionedWriter) WriteValue(name string, value interface{}, saveBookmark ...bool) error {
	if len(saveBookmark) > 0 && saveBookmark[0] {
		currentPosition, err := p.writer.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		p.bookmarks[name] = currentPosition
	}
	if err := WriteValueFunc(p.writer, value); err != nil {
		return ierrors.Wrapf(err, "unable to write value %s", name)
	}

	return nil
}

func (p *PositionedWriter) WriteValueAtBookmark(name string, value interface{}) error {
	bookmarkPosition, ok := p.bookmarks[name]
	if !ok {
		return ierrors.Errorf("unable to find saved position for bookmark %s", name)
	}
	originalPosition, err := p.writer.Seek(0, io.SeekCurrent)
	if err != nil {
		return ierrors.Wrap(err, "unable to obtain current seek position")
	}
	if bookmarkPosition >= originalPosition {
		return ierrors.Errorf("cannot write into the future, current write position %d is greater than or equal to the bookmark position %d", originalPosition, bookmarkPosition)
	}
	if _, err := p.writer.Seek(bookmarkPosition, io.SeekStart); err != nil {
		return ierrors.Wrapf(err, "unable to seek back to bookmark %s position", name)
	}
	if err := WriteValueFunc(p.writer, value); err != nil {
		return ierrors.Wrapf(err, "unable to write value %s", name)
	}
	if _, err := p.writer.Seek(originalPosition, io.SeekStart); err != nil {
		return ierrors.Wrap(err, "unable to seek to original position")
	}

	return nil
}
