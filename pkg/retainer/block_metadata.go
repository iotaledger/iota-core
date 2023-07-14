package retainer

type BlockStatus uint8

const (
	BlockUnknown BlockStatus = iota
	BlockAccepted
	BlockConfirmed
	BlockFinalized
	BlockOrphaned
)

type BlockMetadata struct {
	Status BlockStatus `serix:"0"`
}
