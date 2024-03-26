package sequencing

func maxBlock(block1 *Block, block2 *Block) *Block {
	if block2 == nil {
		return block1
	} else if block1 == nil {
		return block2
	}

	if block1.VirtualVoting.Round > block2.VirtualVoting.Round {
		return block1
	} else if block1.VirtualVoting.Round < block2.VirtualVoting.Round {
		return block2
	}

	if block1.Hash > block2.Hash {
		return block1
	} else {
		return block2
	}
}
