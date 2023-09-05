package tipselectionv1_test

import (
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

func TestTipSelection(t *testing.T) {
	now := time.Now()

	tf := NewTestFramework(t)

	tf.TipManager.CreateBlock("Block1", map[iotago.ParentsType][]string{iotago.StrongParentType: {"Genesis"}}, withIssuingTime(now))
	tf.TipManager.CreateBlock("Block2", map[iotago.ParentsType][]string{iotago.StrongParentType: {"Block1"}})

	tf.TipManager.AddBlock("Block1")
	tf.TipManager.AssertStrongTips("Block1")

	tf.TipManager.AddBlock("Block2")
	tf.TipManager.AssertStrongTips("Block2")

	tf.Instance.SetAcceptanceTime(now.Add(10 * time.Minute))
	tf.TipManager.AssertStrongTips()
}

func withIssuingTime(issuingTime time.Time) func(builder *builder.BasicBlockBuilder) {
	return func(builder *builder.BasicBlockBuilder) {
		builder.IssuingTime(issuingTime)
	}
}
