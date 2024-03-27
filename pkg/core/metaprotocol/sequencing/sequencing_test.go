package sequencing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
)

func TestSequencing(t *testing.T) {
	genesis := NewBlock("genesis", "genesis")
	genesis.Vote.FromGenesisBlock(genesis, ds.NewSet[IssuerID]("validator1", "validator2", "validator3", "validator4"))

	block1 := NewBlock("validator1", "validator1-row1")
	block2 := NewBlock("validator2", "validator2-row1")
	block3 := NewBlock("validator3", "validator3-row1")
	block4 := NewBlock("validator4", "validator4-row1")
	block1.AttachToParents(genesis)
	block2.AttachToParents(genesis)
	block3.AttachToParents(genesis)
	block4.AttachToParents(genesis)

	block5 := NewBlock("validator1", "validator1-row2")
	block6 := NewBlock("validator2", "validator2-row2")
	block7 := NewBlock("validator3", "validator3-row2")
	block8 := NewBlock("validator4", "validator4-row2")
	block5.AttachToParents(block1, block2, block3, block4)
	block6.AttachToParents(block1, block2, block3, block4)
	block7.AttachToParents(block1, block2, block3, block4)
	block8.AttachToParents(block1, block2, block3, block4)

	require.True(t, genesis.Vote.IsMilestone.WasTriggered())

	block9 := NewBlock("validator1", "validator1-row3")
	block10 := NewBlock("validator2", "validator2-row3")
	block11 := NewBlock("validator3", "validator3-row3")
	block12 := NewBlock("validator4", "validator4-row3")
	block9.AttachToParents(block5, block6, block7, block8)
	block10.AttachToParents(block5, block6, block7, block8)
	block11.AttachToParents(block5, block6, block7, block8)
	block12.AttachToParents(block5, block6, block7, block8)

	require.True(t, block1.Vote.IsMilestone.WasTriggered())

	block13 := NewBlock("validator1", "validator1-row4")
	block14 := NewBlock("validator2", "validator2-row4")
	block15 := NewBlock("validator3", "validator3-row4")
	block16 := NewBlock("validator4", "validator4-row4")
	block13.AttachToParents(block9, block10, block11, block12)
	block14.AttachToParents(block9, block10, block11, block12)
	block15.AttachToParents(block9, block10, block11, block12)
	block16.AttachToParents(block9, block10, block11, block12)

	require.True(t, block8.Vote.IsMilestone.WasTriggered())

	block17 := NewBlock("validator1", "validator1-row5")
	block18 := NewBlock("validator2", "validator2-row5")
	block19 := NewBlock("validator3", "validator3-row5")
	block20 := NewBlock("validator4", "validator4-row5")
	block17.AttachToParents(block13, block14, block15, block16)
	block18.AttachToParents(block13, block14, block15, block16)
	block19.AttachToParents(block13, block14, block15, block16)
	block20.AttachToParents(block13, block14, block15, block16)

	require.True(t, block11.Vote.IsMilestone.WasTriggered())

}
