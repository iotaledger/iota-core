//go:build dockertests

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_SmallerCommittee(t *testing.T) {
	fmt.Println("Test_SmallerCommittee")
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
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

	d.AssertCommittee(currentEpoch+2, d.AccountsFromNodes(d.Nodes("V1", "V3", "V4")...))

	// restart validator 2
	err = d.RestartContainer(d.Node("V2").ContainerName)
	require.NoError(t, err)

	d.AssertCommittee(currentEpoch+3, d.AccountsFromNodes(d.Nodes()...))
}

func Test_ReuseDueToNoFinalization(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
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
	d.AssertCommittee(currentEpoch+2, d.AccountsFromNodes(d.Nodes()...))

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

	d.AssertCommittee(currentEpoch+3, d.AccountsFromNodes(d.Nodes("V1", "V2", "V4")...))

	// wait finalization to catch up and check if the finalization resumes
	time.Sleep(5 * time.Second)
	d.AssertFinalizedSlot(func(newFinalizedSlot iotago.SlotIndex) error {
		if prevFinalizedSlot < newFinalizedSlot {
			return nil
		}

		return ierrors.Errorf("Finalization should happened, Second finalized slot: %d, Third finalized slot: %d", prevFinalizedSlot, status.LatestFinalizedSlot)
	})
}

func Test_NoCandidacyPayload(t *testing.T) {
	fmt.Println("Test_NoCandidacyPayload")
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	err = d.WaitUntilSync()
	require.NoError(t, err)

	clt := d.Node("V1").Client
	status := d.NodeStatus("V1")
	prevFinalizedSlot := status.LatestFinalizedSlot
	fmt.Println("First finalized slot: ", prevFinalizedSlot)
	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)

	d.StopIssueCandidacyPayload(d.Nodes()...)

	// Due to no candidacy payloads, committee should be reused, remain 4 validators
	d.AssertCommittee(currentEpoch+2, d.AccountsFromNodes(d.Nodes()...))

	// check if finalization continues
	fmt.Println("Second finalized slot: ", status.LatestFinalizedSlot)
	d.AssertFinalizedSlot(func(newFinalizedSlot iotago.SlotIndex) error {
		if prevFinalizedSlot < newFinalizedSlot {
			return nil
		}

		return ierrors.Errorf("Finalization should happened, First finalized slot: %d, Second finalized slot: %d", prevFinalizedSlot, status.LatestFinalizedSlot)
	})

	// Start issuing candidacy payloads for 3 validators, and check if committee size is 3
	d.StartIssueCandidacyPayload(d.Nodes("V1", "V2", "V3")...)
	d.AssertCommittee(currentEpoch+4, d.AccountsFromNodes(d.Nodes("V1", "V2", "V3")...))
}

func Test_Staking(t *testing.T) {
	fmt.Println("Test_Staking")
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1),
			iotago.WithTargetCommitteeSize(3),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	err = d.WaitUntilSync()
	require.NoError(t, err)

	accountAddr := d.CreateAccount(WithStakingFeature(100, 1, 0))

	d.AssertValidatorExists(accountAddr)
}
