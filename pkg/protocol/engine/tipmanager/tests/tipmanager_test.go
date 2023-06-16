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
