package tipmanagerv1

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/model"
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

	tf.AddBlock("Bernd")
	tf.AssertStrongTips("Bernd")

	tf.AddBlock("Bernd1")
	tf.AssertStrongTips("Bernd1")

	tf.AddBlock("Bernd1.1")
	tf.AssertStrongTips("Bernd1", "Bernd1.1")
}
