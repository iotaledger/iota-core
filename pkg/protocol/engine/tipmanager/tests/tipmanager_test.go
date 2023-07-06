package tests

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestTipManager(t *testing.T) {
	tf := NewTestFramework(t)

	tf.CreateBlock("Bernd", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Genesis"},
	})
	tf.CreateBlock("Bernd1", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Bernd"},
	})
	tf.CreateBlock("Bernd1.1", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Bernd"},
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

	tf.CreateBlock("A", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Genesis"},
	})
	tf.CreateBlock("B", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Genesis"},
	})
	tf.CreateBlock("C", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"A", "B"},
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
