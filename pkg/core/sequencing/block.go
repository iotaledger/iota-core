package sequencing

import "sync/atomic"

type Block struct {
	ID     Identifier
	Issuer Identifier
	Hash   uint64

	VirtualVoting *VirtualVoting
}

func NewBlock(issuer Identifier, id Identifier) *Block {
	b := &Block{
		ID:     id,
		Issuer: issuer,
		Hash:   hashCounter.Add(1),
	}

	b.VirtualVoting = NewVirtualVoting(b)

	return b
}

func (b *Block) AttachToParents(parents ...*Block) {
	b.VirtualVoting.ProcessVote(b.Issuer, parents...)
}

var hashCounter atomic.Uint64
