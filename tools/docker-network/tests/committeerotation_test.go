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

// Test_SmallerCommittee tests if the committee rotated to a smaller committee than targetCommitteeSize
// if less than targetCommitteeSize validators issued candidacy payloads.
// 1. Run docker network, targetCommitteeSize=4, with 4 validators running.
// 2. Shut down inx-validator of V2.
// 3. Check that committee of size 3 is selected in next epoch.
// 4. Restart inx-validator of V2.
// 5. Check that committee of size 4 is selected in next epoch.
func Test_SmallerCommittee(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1, 384),
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

	// stop inx-validator plugin of validator 2
	err = d.StopContainer(d.Node("V2").ContainerName)
	require.NoError(t, err)

	d.AssertCommittee(currentEpoch+2, d.AccountsFromNodes(d.Nodes("V1", "V3", "V4")...))

	// restart inx-validator plugin of validator 2
	err = d.RestartContainer(d.Node("V2").ContainerName)
	require.NoError(t, err)

	d.AssertCommittee(currentEpoch+3, d.AccountsFromNodes(d.Nodes()...))
}

// Test_ReuseDueToNoFinalization tests if the committee members are the same (reused) due to no slot finalization at epochNearingThreshold and recovery after finalization comes back.
// 1. Run docker network, targetCommitteeSize=4, with 4 validators running.
// 2. Shutdown inx-validator of V2 and V3.
// 3. Check if finalization stops and committee is reused (remains 4 committee members) in next epoch due to no finalization.
// 4. Restart inx-validator of V2.
// 5. Check that committee of size 3 (V1, V2, V4) is selected in next epoch and finalization occurs again from that epoch.
func Test_ReuseDueToNoFinalization(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1, 384),
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

// Test_NoCandidacyPayload tests if committee is reused due to no candidates announced but slot finalized at epochNearingThreshold.
// 1. Run docker network, targetCommitteeSize=4, with 4 validators running.
// 2. Stop issuing candidacy payload on all validators.
// 3. Check finalization advances and the committee is reused in next epoch due to no candidates.
// 4. Start issuing candidacy payload on 3 validators only.
// 5. Check finalization advances and the committee is changed to 3 committee members.
func Test_NoCandidacyPayload(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1, 384),
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
	d.AssertFinalizedSlot(func(newFinalizedSlot iotago.SlotIndex) error {
		if prevFinalizedSlot < newFinalizedSlot {
			return nil
		}

		return ierrors.Errorf("Finalization should happened, First finalized slot: %d, Second finalized slot: %d", prevFinalizedSlot, newFinalizedSlot)
	})

	// Start issuing candidacy payloads for 3 validators, and check if committee size is 3
	d.StartIssueCandidacyPayload(d.Nodes("V1", "V2", "V3")...)
	d.AssertCommittee(currentEpoch+4, d.AccountsFromNodes(d.Nodes("V1", "V2", "V3")...))
}

// Test_Staking tests if an newly created account becomes a staker with staking feature.
// 1. Run docker network, targetCommitteeSize=3, with 4 validators running.
// 2. Create an account with staking feature.
// 3. Check if the account became a staker.
func Test_Staking(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1, 384),
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

	account := d.CreateAccount(WithStakingFeature(100, 1, 0))

	d.AssertValidatorExists(account.AccountAddress)
}

// Test_Delegation tests if committee changed due to delegation.
// initial stake: V1 > V2 > V3 > V4
// 1. Run docker network, targetCommitteeSize=3, with 4 validators running. Committee members are: V1, V2, V3
// 2. Create an account for delegation.
// 3. Delegate requested faucet funds to V4, V4 should replace V3 as a committee member. (V4 > V1 > V2 > V3)
// 4. Delegate requested faucet funds to V3, V3 should replace V2 as a committee member. (V3 > V4 > V1 > V2)
func Test_Delegation(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1, 384),
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

	// create an account to perform delegation
	account := d.CreateAccount()

	// delegate all faucet funds to V4, V4 should replace V3
	delegationStartEpoch := d.DelegateToValidator(account, d.Node("V4"))
	d.AssertCommittee(delegationStartEpoch+1, d.AccountsFromNodes(d.Nodes("V1", "V2", "V4")...))

	// delegate all faucet funds to V3, V3 should replace V2
	delegationStartEpoch = d.DelegateToValidator(account, d.Node("V3"))
	d.AssertCommittee(delegationStartEpoch+1, d.AccountsFromNodes(d.Nodes("V1", "V3", "V4")...))
}
