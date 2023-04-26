package permanent

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/storable"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitments struct {
	api      iotago.API
	slice    *storable.ByteSlice
	filePath string

	module.Module
}

func NewCommitments(path string, api iotago.API) *Commitments {
	commitmentsSlice, err := storable.NewByteSlice(path, uint64(len(model.NewEmptyCommitment(api).Data())))
	if err != nil {
		panic(errors.Wrap(err, "failed to create commitments file"))
	}

	return &Commitments{
		api:      api,
		slice:    commitmentsSlice,
		filePath: path,
	}
}

func (c *Commitments) Store(commitment *model.Commitment) (err error) {
	if err = c.slice.Set(uint64(commitment.Commitment().Index), commitment.Data()); err != nil {
		return errors.Wrapf(err, "failed to store commitment for slot %d", commitment.Commitment().Index)
	}

	return nil
}

func (c *Commitments) loadBytes(index iotago.SlotIndex) (commitmentBytes []byte, err error) {
	return c.slice.Get(uint64(index))
}

func (c *Commitments) Load(index iotago.SlotIndex) (commitment *model.Commitment, err error) {
	bytes, err := c.loadBytes(index)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get commitment for slot %d", index)
	}

	return model.CommitmentFromBytes(bytes, c.api)
}

func (c *Commitments) Close() (err error) {
	return c.slice.Close()
}

// FilePath returns the path that this is associated to.
func (c *Commitments) FilePath() (filePath string) {
	return c.filePath
}

func (c *Commitments) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	if err = binary.Write(writer, binary.LittleEndian, int64(targetSlot)); err != nil {
		return errors.Wrap(err, "failed to write slot boundary")
	}

	for slotIndex := iotago.SlotIndex(0); slotIndex <= targetSlot; slotIndex++ {
		commitmentBytes, err := c.loadBytes(slotIndex)
		if err != nil {
			return errors.Wrapf(err, "failed to load commitment for slot %d", slotIndex)
		}
		if err = binary.Write(writer, binary.LittleEndian, commitmentBytes); err != nil {
			return errors.Wrapf(err, "failed to write commitment for slot %d", slotIndex)
		}
	}

	return nil
}

func (c *Commitments) Import(reader io.ReadSeeker) (err error) {
	var slotBoundary int64
	if err = binary.Read(reader, binary.LittleEndian, &slotBoundary); err != nil {
		return errors.Wrap(err, "failed to read slot boundary")
	}

	commitmentSize := c.slice.EntrySize()

	for slotIndex := int64(0); slotIndex <= slotBoundary; slotIndex++ {
		commitmentBytes := make([]byte, commitmentSize)
		if err = binary.Read(reader, binary.LittleEndian, commitmentBytes); err != nil {
			return errors.Wrapf(err, "failed to read commitment bytes for slot %d", slotIndex)
		}

		newCommitment, err := model.CommitmentFromBytes(commitmentBytes, c.api)
		if err != nil {
			return errors.Wrapf(err, "failed to parse commitment of slot %d", slotIndex)
		}

		if err = c.Store(newCommitment); err != nil {
			return errors.Wrapf(err, "failed to store commitment of slot %d", slotIndex)
		}
	}

	c.TriggerInitialized()

	return nil
}
