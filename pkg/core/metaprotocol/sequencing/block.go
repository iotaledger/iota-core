package sequencing

import (
	"fmt"
	"sync/atomic"
)

// Block is a block in the block DAG.
type Block struct {
	// ID is the identifier of the block.
	ID IssuerID

	// Issuer is the identifier of the issuer of the block.
	Issuer IssuerID

	// Vote is the vote of the block.
	Vote *Vote

	// Hash is a piece of data that is used to make the Hash unique.
	Hash uint64
}

// NewBlock creates a new block with the given issuer and id.
func NewBlock(issuer IssuerID, id IssuerID) *Block {
	b := &Block{
		ID:     id,
		Issuer: issuer,
		Vote:   NewVote(),
		Hash:   hashCounter.Add(1),
	}

	b.Vote.IsMilestone.OnTrigger(func() {
		fmt.Println("Milestone accepted:", b.ID)
	})

	return b
}

// AttachToParents attaches the block to the given parent blocks.
func (b *Block) AttachToParents(parents ...*Block) {
	b.Vote.FromBlockContext(b, parents...)
}

// hashCounter is a counter that is used to generate unique hashes for blocks.
var hashCounter atomic.Uint64
