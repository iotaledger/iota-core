package tests

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
)

func TestTipManager(t *testing.T) {
	tf := NewTestFramework(t)

	tf.CreateBlock("Bernd", map[model.ParentsType][]string{
		model.StrongParentType: {"Genesis"},
	})
	tf.CreateBlock("Bernd1", map[model.ParentsType][]string{
		model.StrongParentType: {"Bernd"},
	})
	tf.CreateBlock("Bernd1.1", map[model.ParentsType][]string{
		model.StrongParentType: {"Bernd"},
	})

	tf.AddBlock("Bernd").SetTipPool(tipmanager.StrongTipPool)
	tf.AssertStrongTips("Bernd")

	tf.AddBlock("Bernd1").SetTipPool(tipmanager.StrongTipPool)
	tf.AssertStrongTips("Bernd1")

	tf.AddBlock("Bernd1.1").SetTipPool(tipmanager.StrongTipPool)
	tf.AssertStrongTips("Bernd1", "Bernd1.1")
}

func Test_Orphanage(t *testing.T) {
	tf := NewTestFramework(t)

	tf.CreateBlock("A", map[model.ParentsType][]string{
		model.StrongParentType: {"Genesis"},
	})
	tf.CreateBlock("B", map[model.ParentsType][]string{
		model.StrongParentType: {"Genesis"},
	})
	tf.CreateBlock("C", map[model.ParentsType][]string{
		model.StrongParentType: {"A", "B"},
	})

	tf.AddBlock("A").SetTipPool(tipmanager.StrongTipPool)
	tf.AssertStrongTips("A")

	blockB := tf.AddBlock("B")
	blockB.SetTipPool(tipmanager.StrongTipPool)
	tf.AssertStrongTips("A", "B")

	tf.AddBlock("C").SetTipPool(tipmanager.StrongTipPool)
	tf.AssertStrongTips("C")

	blockB.SetMarkedOrphaned(true)
	tf.AssertStrongTips("A")
}
