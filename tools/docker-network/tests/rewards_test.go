//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/stretchr/testify/require"
)

// Test_ValidatorRewards tests the rewards for a validator.
// 1. Create an account with staking feature.
// 2. Issue candidacy payloads for the account and wait until the account is in the committee.
// 3. Issue validation blocks until claiming slot is reached.
// 4. Claim rewards and check if the mana increased as expected.
func Test_ValidatorRewards(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithStakingOptions(3, 10, 10),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	d.WaitUntilNetworkReady()

	ctx := context.Background()
	clt := d.wallet.DefaultClient()
	status := d.NodeStatus("V1")
	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)

	// Set end epoch so the staking feature can be removed as soon as possible.
	endEpoch := currentEpoch + clt.CommittedAPI().ProtocolParameters().StakingUnbondingPeriod()
	// The earliest epoch in which we can remove the staking feature and claim rewards.
	claimingSlot := clt.CommittedAPI().TimeProvider().EpochStart(endEpoch + 1)

	account := d.CreateAccount(WithStakingFeature(100, 1, currentEpoch, endEpoch))
	initialMana := account.Output.StoredMana()

	// continue issuing candidacy payload for account in the background
	go func() {
		fmt.Println("Issuing candidacy payloads for account in the background...")
		defer fmt.Println("Issuing candidacy payloads for account in the background......done")

		currentSlot := clt.CommittedAPI().TimeProvider().CurrentSlot()

		for i := currentSlot; i < claimingSlot; i++ {
			d.IssueCandidacyPayloadFromAccount(account.ID)
			time.Sleep(10 * time.Second)
		}
	}()

	// make sure the account is in the committee, so it can issue validation blocks
	accountAddrBech32 := account.Address.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())
	d.AssertCommittee(currentEpoch+2, append(d.AccountsFromNodes(d.Nodes("V1", "V3", "V2", "V4")...), accountAddrBech32))

	// issua validation blocks to have performance
	currentSlot := clt.CommittedAPI().TimeProvider().CurrentSlot()
	slotToWait := claimingSlot - currentSlot
	secToWait := time.Duration(slotToWait) * time.Duration(clt.CommittedAPI().TimeProvider().SlotDurationSeconds()) * time.Second
	fmt.Println("Wait for ", secToWait, "until expected slot: ", claimingSlot)

	for i := currentSlot; i < claimingSlot; i++ {
		d.SubmitValidationBlock(account.ID)
		time.Sleep(10 * time.Second)
	}

	// claim rewards that put to the account output
	account = d.ClaimRewardsForValidator(ctx, account)

	// check if the mana increased as expected
	outputFromAPI, err := clt.OutputByID(ctx, account.OutputID)
	require.NoError(t, err)
	require.Greater(t, outputFromAPI.StoredMana(), initialMana)
	require.Equal(t, account.Output.StoredMana(), outputFromAPI.StoredMana())
}
