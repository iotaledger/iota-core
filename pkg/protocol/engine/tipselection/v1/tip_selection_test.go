package tipselectionv1_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestTipSelection_DynamicLivenessThreshold_NoWitnesses(t *testing.T) {
	tf := NewTestFramework(t)
	tf.TipManager.CreateBasicBlock("Block", map[iotago.ParentsType][]string{iotago.StrongParentType: {"Genesis"}})
	tf.TipManager.AddBlock("Block")

	expectedLivenessThreshold := tf.ExpectedLivenessThreshold("Block")
	require.Equal(t, tf.LowerLivenessThreshold("Block"), expectedLivenessThreshold)

	// assert initial state
	{
		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to just before liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThreshold.Add(-1))

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to reach liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThreshold)

		tf.TipManager.RequireLivenessThresholdReached("Block", true)
		tf.TipManager.RequireStrongTips()
	}
}

func TestTipSelection_DynamicLivenessThreshold_WithSingleWitness(t *testing.T) {
	tf := NewTestFramework(t)
	tf.TipManager.CreateBasicBlock("Block", map[iotago.ParentsType][]string{iotago.StrongParentType: {"Genesis"}})
	tf.TipManager.AddBlock("Block")

	expectedLivenessThresholdWithoutWitnesses := tf.ExpectedLivenessThreshold("Block")
	require.Equal(t, tf.LowerLivenessThreshold("Block"), expectedLivenessThresholdWithoutWitnesses)

	// assert initial state
	{
		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to just before liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThresholdWithoutWitnesses.Add(-1))

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// add witness
	tf.TipManager.Block("Block").AddWitness(0)
	expectedLivenessThresholdWithOneWitness := tf.ExpectedLivenessThreshold("Block")
	require.Less(t, expectedLivenessThresholdWithoutWitnesses.Unix(), expectedLivenessThresholdWithOneWitness.Unix())

	// advance time to reach previous liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThresholdWithoutWitnesses)

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to just before new liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThresholdWithOneWitness.Add(-1))

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to reach new liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThresholdWithOneWitness)

		tf.TipManager.RequireLivenessThresholdReached("Block", true)
		tf.TipManager.RequireStrongTips()
	}
}

func TestTipSelection_DynamicLivenessThreshold_WithMaxWitnesses(t *testing.T) {
	tf := NewTestFramework(t)
	tf.TipManager.CreateBasicBlock("Block", map[iotago.ParentsType][]string{iotago.StrongParentType: {"Genesis"}})
	tf.TipManager.AddBlock("Block")

	livenessThresholdZero := tf.ExpectedLivenessThreshold("Block")
	require.Equal(t, tf.LowerLivenessThreshold("Block"), livenessThresholdZero)

	// assert initial state
	{
		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to just before liveness threshold
	{
		tf.Instance.SetAcceptanceTime(livenessThresholdZero.Add(-1))

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// add witnesses (not enough to reach max)
	tf.TipManager.Block("Block").AddWitness(0)
	tf.TipManager.Block("Block").AddWitness(1)
	tf.TipManager.Block("Block").AddWitness(2)
	livenessThresholdThree := tf.ExpectedLivenessThreshold("Block")
	require.Less(t, livenessThresholdZero.Unix(), livenessThresholdThree.Unix())

	// advance time to reach previous liveness threshold
	{
		tf.Instance.SetAcceptanceTime(livenessThresholdZero)

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to just before new liveness threshold
	{
		tf.Instance.SetAcceptanceTime(livenessThresholdThree.Add(-1))

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// add witness (reaches max)
	tf.TipManager.Block("Block").AddWitness(3)
	livenessThresholdFour := tf.ExpectedLivenessThreshold("Block")
	require.Less(t, livenessThresholdThree.Unix(), livenessThresholdFour.Unix())
	require.Equal(t, tf.UpperLivenessThreshold("Block"), livenessThresholdFour)

	// advance time to reach previous liveness threshold
	{
		tf.Instance.SetAcceptanceTime(livenessThresholdThree)

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// advance time to just before new liveness threshold
	{
		tf.Instance.SetAcceptanceTime(livenessThresholdFour.Add(-1))

		tf.TipManager.RequireLivenessThresholdReached("Block", false)
		tf.TipManager.RequireStrongTips("Block")
	}

	// add witness (reaches above max)
	tf.TipManager.Block("Block").AddWitness(4)
	expectedLivenessThresholdWithFiveWitness := tf.ExpectedLivenessThreshold("Block")
	require.Equal(t, livenessThresholdFour, expectedLivenessThresholdWithFiveWitness)
	require.Equal(t, tf.UpperLivenessThreshold("Block"), livenessThresholdFour)

	// advance time to reach new liveness threshold
	{
		tf.Instance.SetAcceptanceTime(expectedLivenessThresholdWithFiveWitness)

		tf.TipManager.RequireLivenessThresholdReached("Block", true)
		tf.TipManager.RequireStrongTips()
	}
}

func TestDynamicLivenessThreshold(t *testing.T) {
	const committeeSize = 10
	tf := NewTestFramework(t, WithCommitteeSize(committeeSize))
	tf.TipManager.CreateBasicBlock("Block", map[iotago.ParentsType][]string{iotago.StrongParentType: {"Genesis"}})
	tf.TipManager.AddBlock("Block")

	livenessThresholdLowerBound := tf.TipManager.API.ProtocolParameters().LivenessThresholdLowerBound()
	livenessWindow := float64(tf.TipManager.API.ProtocolParameters().LivenessThresholdUpperBound() - livenessThresholdLowerBound)

	tf.RequireLivenessThreshold("Block", livenessThresholdLowerBound)

	tf.TipManager.Block("Block").AddWitness(0)
	tf.RequireLivenessThreshold("Block", livenessThresholdLowerBound+time.Duration(approvalModifier(1, committeeSize)*livenessWindow))

	tf.TipManager.Block("Block").AddWitness(1)
	tf.RequireLivenessThreshold("Block", livenessThresholdLowerBound+time.Duration(approvalModifier(2, committeeSize)*livenessWindow))

	tf.TipManager.Block("Block").AddWitness(2)
	tf.RequireLivenessThreshold("Block", livenessThresholdLowerBound+time.Duration(approvalModifier(3, committeeSize)*livenessWindow))

	// We've reached > 1/3 -> liveness threshold should be at upper bound
	tf.TipManager.Block("Block").AddWitness(3)
	tf.RequireLivenessThreshold("Block", tf.TipManager.API.ProtocolParameters().LivenessThresholdUpperBound())

	tf.TipManager.Block("Block").AddWitness(4)
	tf.TipManager.Block("Block").AddWitness(5)
	tf.TipManager.Block("Block").AddWitness(6)
	tf.TipManager.Block("Block").AddWitness(7)
	tf.TipManager.Block("Block").AddWitness(8)
	tf.TipManager.Block("Block").AddWitness(9)
	tf.RequireLivenessThreshold("Block", tf.TipManager.API.ProtocolParameters().LivenessThresholdUpperBound())
}

func approvalModifier(witnessCount float64, committeeSize float64) float64 {
	return witnessCount / math.Ceil(committeeSize/3.0)
}
