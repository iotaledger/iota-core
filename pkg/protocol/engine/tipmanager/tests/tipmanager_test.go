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

	tf.AddBlock("Bernd").TipPool().Set(tipmanager.StrongTipPool)
	tf.AssertStrongTips("Bernd")

	tf.AddBlock("Bernd1").TipPool().Set(tipmanager.StrongTipPool)
	tf.AssertStrongTips("Bernd1")

	tf.AddBlock("Bernd1.1").TipPool().Set(tipmanager.StrongTipPool)
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

	tf.AddBlock("A").TipPool().Set(tipmanager.StrongTipPool)
	tf.AssertStrongTips("A")

	blockB := tf.AddBlock("B")
	blockB.TipPool().Set(tipmanager.StrongTipPool)
	tf.AssertStrongTips("A", "B")

	tf.AddBlock("C").TipPool().Set(tipmanager.StrongTipPool)
	tf.AssertStrongTips("C")

	blockB.LivenessThresholdReached().Trigger()
	tf.AssertStrongTips("A")
}
