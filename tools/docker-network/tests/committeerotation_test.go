package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/stretchr/testify/require"
)

func Test_SmallerCommittee(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050")
	d.AddNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060")
	d.AddNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070")
	d.AddNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	err = d.WaitUntilSync()
	require.NoError(t, err)

	status := d.NodeStatus("V1")

	clt := d.Node("V1").Client
	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)

	// stop validator 2
	err = d.StopContainer(d.Node("V2").ContainerName)
	require.NoError(t, err)

	d.AssertCommitteeSize(currentEpoch+2, 3)

	// restart validator 2
	err = d.RestartContainer(d.Node("V2").ContainerName)
	require.NoError(t, err)

	d.AssertCommitteeSize(currentEpoch+3, 4)
}

func Test_ReuseDueToNoFinalization(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(30, 30, 2, 4, 8),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050")
	d.AddNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060")
	d.AddNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070")
	d.AddNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	err = d.WaitUntilSync()
	require.NoError(t, err)

	// stop 2 validators, finalization should stop
	err = d.StopContainer(d.Node("V2").ContainerName, d.Node("V3").ContainerName)
	require.NoError(t, err)

	clt := d.Node("V1").Client
	status := d.NodeStatus("V1")

	prevFinalizedSlot := status.LatestFinalizedSlot
	fmt.Println("First finalized slot: ", prevFinalizedSlot)

	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(prevFinalizedSlot)

	// Due to no finalization, committee should be reused, remain 4 validators
	d.AssertCommitteeSize(currentEpoch+2, 4)

	// check if finalization stops
	fmt.Println("Second finalized slot: ", status.LatestFinalizedSlot)
	d.AssertFinalizedSlot(func(newFinalizedSlot iotago.SlotIndex) error {
		if prevFinalizedSlot == newFinalizedSlot {
			return nil
		}

		return ierrors.Errorf("NO finalization should happened, First finalized slot: %d, Second finalized slot: %d", prevFinalizedSlot, status.LatestFinalizedSlot)
	})

	// revive 1 validator, committee size should be 3, finalization should resume
	err = d.RestartContainer(d.Node("V2").ContainerName)
	require.NoError(t, err)

	d.AssertCommitteeSize(currentEpoch+3, 3)

	// wait finalization to catch up and check if the finalization resumes
	time.Sleep(5 * time.Second)
	d.AssertFinalizedSlot(func(newFinalizedSlot iotago.SlotIndex) error {
		if prevFinalizedSlot < newFinalizedSlot {
			return nil
		}

		return ierrors.Errorf("Finalization should happened, Second finalized slot: %d, Third finalized slot: %d", prevFinalizedSlot, status.LatestFinalizedSlot)
	})
}
