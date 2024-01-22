package tests

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestTipManager(t *testing.T) {
	tf := NewTestFramework(t)

	tf.CreateBasicBlock("Bernd", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Genesis"},
	})
	tf.CreateBasicBlock("Bernd1", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Bernd"},
	})
	tf.CreateBasicBlock("Bernd1.1", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Bernd"},
	})

	tf.AddBlock("Bernd").TipPool().Set(tipmanager.StrongTipPool)
	tf.RequireStrongTips("Bernd")

	tf.AddBlock("Bernd1").TipPool().Set(tipmanager.StrongTipPool)
	tf.RequireStrongTips("Bernd1")

	tf.AddBlock("Bernd1.1").TipPool().Set(tipmanager.StrongTipPool)
	tf.RequireStrongTips("Bernd1", "Bernd1.1")
}

func Test_Orphanage(t *testing.T) {
	tf := NewTestFramework(t)

	tf.CreateBasicBlock("A", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Genesis"},
	})
	tf.CreateBasicBlock("B", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"Genesis"},
	})
	tf.CreateBasicBlock("C", map[iotago.ParentsType][]string{
		iotago.StrongParentType: {"A", "B"},
	})

	tf.AddBlock("A").TipPool().Set(tipmanager.StrongTipPool)
	tf.RequireStrongTips("A")

	blockB := tf.AddBlock("B")
	blockB.TipPool().Set(tipmanager.StrongTipPool)
	tf.RequireStrongTips("A", "B")

	tf.AddBlock("C").TipPool().Set(tipmanager.StrongTipPool)
	tf.RequireStrongTips("C")

	blockB.LivenessThresholdReached().Trigger()
	tf.RequireStrongTips("A")
}

func Test_ValidationTips(t *testing.T) {
	tf := NewTestFramework(t)

	tf.AddValidators("validatorA", "validatorB")

	{
		tf.CreateBasicBlock("1", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"Genesis"},
		})
		tf.CreateBasicBlock("2", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"Genesis"},
		})

		tf.AddBlock("1").TipPool().Set(tipmanager.StrongTipPool)
		tf.AddBlock("2").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireStrongTips("1", "2")
	}

	// Add validation tip for validatorA.
	{
		tf.CreateValidationBlock("3", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"2"},
		}, func(blockBuilder *builder.ValidationBlockBuilder) {
			blockBuilder.Sign(tf.Validator("validatorA"), tpkg.RandEd25519PrivateKey())
		})

		tf.AddBlock("3").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireValidationTips("3")
		tf.RequireStrongTips("1", "3")
	}

	// Add validation tip for validatorB.
	{
		tf.CreateValidationBlock("4", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"1"},
		}, func(blockBuilder *builder.ValidationBlockBuilder) {
			blockBuilder.Sign(tf.Validator("validatorB"), tpkg.RandEd25519PrivateKey())
		})

		tf.AddBlock("4").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireValidationTips("3", "4")
		tf.RequireStrongTips("3", "4")
	}

	// Add basic blocks in the future cone of the validation tips, referencing both existing validation tips.
	{
		tf.CreateBasicBlock("5", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"3", "4"},
		})

		tf.AddBlock("5").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireValidationTips("5")
		tf.RequireStrongTips("5")
	}

	// Add basic blocks in the future cone of the validation tips.
	{
		tf.CreateBasicBlock("6", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"3"},
		})
		tf.CreateBasicBlock("7", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"4"},
		})

		tf.AddBlock("6").TipPool().Set(tipmanager.StrongTipPool)
		tf.AddBlock("7").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireValidationTips("5", "6", "7")
		tf.RequireStrongTips("5", "6", "7")
	}

	// A newer validation block replaces the previous validation tip of that validator.
	{
		tf.CreateValidationBlock("8", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"6"},
		}, func(blockBuilder *builder.ValidationBlockBuilder) {
			blockBuilder.Sign(tf.Validator("validatorB"), tpkg.RandEd25519PrivateKey())
		})

		tf.AddBlock("8").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireValidationTips("5", "8")
		tf.RequireStrongTips("5", "7", "8")
	}

	// A newer validation block (with parallel past cone) still becomes a validation tip and overwrites
	// the previous validation tip of that validator.
	{
		tf.CreateValidationBlock("9", map[iotago.ParentsType][]string{
			iotago.StrongParentType: {"Genesis"},
		}, func(blockBuilder *builder.ValidationBlockBuilder) {
			blockBuilder.Sign(tf.Validator("validatorA"), tpkg.RandEd25519PrivateKey())
		})

		tf.AddBlock("9").TipPool().Set(tipmanager.StrongTipPool)

		tf.RequireValidationTips("8", "9")
		tf.RequireStrongTips("5", "7", "8", "9")
	}
}
