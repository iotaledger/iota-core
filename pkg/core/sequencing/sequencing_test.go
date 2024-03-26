package sequencing

import "testing"

func Test(t *testing.T) {
	genesis := NewBlock("genesis", "genesis")
	genesis.VirtualVoting.AddValidator("validator1")
	genesis.VirtualVoting.AddValidator("validator2")
	genesis.VirtualVoting.AddValidator("validator3")
	genesis.VirtualVoting.AddValidator("validator4")

	block1 := NewBlock("validator1", "validator1-row1")
	block1.AttachToParents(genesis)
	block2 := NewBlock("validator2", "validator2-row1")
	block2.AttachToParents(genesis)
	block3 := NewBlock("validator3", "validator3-row1")
	block3.AttachToParents(genesis)
	block4 := NewBlock("validator4", "validator4-row1")
	block4.AttachToParents(genesis)

	block5 := NewBlock("validator1", "validator1-row2")
	block5.AttachToParents(block1, block2, block3)
	block6 := NewBlock("validator2", "validator2-row2")
	block6.AttachToParents(block1, block2, block3)
	block7 := NewBlock("validator3", "validator3-row2")
	block7.AttachToParents(block1, block2, block3)
	block8 := NewBlock("validator4", "validator4-row2")
	block8.AttachToParents(block4, block1, block2, block3)

	block9 := NewBlock("validator1", "validator1-row3")
	block9.AttachToParents(block5, block6, block7, block8)
	block10 := NewBlock("validator2", "validator2-row3")
	block10.AttachToParents(block5, block6, block7, block8)
	block11 := NewBlock("validator3", "validator3-row3")
	block11.AttachToParents(block5, block6, block7, block8)
	block12 := NewBlock("validator4", "validator4-row3")
	block12.AttachToParents(block5, block6, block7, block8)

	block13 := NewBlock("validator1", "validator1-row4")
	block13.AttachToParents(block9, block10, block11, block12)
	block14 := NewBlock("validator2", "validator2-row4")
	block14.AttachToParents(block9, block10, block11, block12)
	block15 := NewBlock("validator3", "validator3-row4")
	block15.AttachToParents(block9, block10, block11, block12)
	block16 := NewBlock("validator4", "validator4-row4")
	block16.AttachToParents(block9, block10, block11, block12)
}
