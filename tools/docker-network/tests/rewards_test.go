//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Test_ValidatorRewards tests the rewards for a validator.
// 1. Create 2 accounts with staking feature.
// 2. Issue candidacy payloads for the accounts and wait until the accounts is in the committee.
// 3. One of the account issues 3 validation blocks per slot, the other account issues 1 validation block per slot until claiming slot is reached.
// 4. Claim rewards and check if the mana increased as expected, the account that issued less validation blocks should have less mana.
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
	clt := d.defaultWallet.Client
	status := d.NodeStatus("V1")
	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)
	slotsDuration := clt.CommittedAPI().ProtocolParameters().SlotDurationInSeconds()

	// Set end epoch so the staking feature can be removed as soon as possible.
	endEpoch := currentEpoch + clt.CommittedAPI().ProtocolParameters().StakingUnbondingPeriod() + 1
	// The earliest epoch in which we can remove the staking feature and claim rewards.
	claimingSlot := clt.CommittedAPI().TimeProvider().EpochStart(endEpoch + 1)

	// create accounts and continue issuing candidacy payload for account in the background
	goodWallet, goodAccountData := d.CreateAccount(WithStakingFeature(100, 1, currentEpoch, endEpoch))
	initialMana := goodAccountData.Output.StoredMana()
	issueCandidacyPayloadInBackground(d, goodWallet, clt.CommittedAPI().TimeProvider().CurrentSlot(), claimingSlot,
		slotsDuration)

	lazyWallet, lazyAccountData := d.CreateAccount(WithStakingFeature(100, 1, currentEpoch, endEpoch))
	lazyInitialMana := lazyAccountData.Output.StoredMana()
	issueCandidacyPayloadInBackground(d, lazyWallet, clt.CommittedAPI().TimeProvider().CurrentSlot(), claimingSlot,
		slotsDuration)

	// make sure the account is in the committee, so it can issue validation blocks
	goodAccountAddrBech32 := goodAccountData.Address.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())
	lazyAccountAddrBech32 := lazyAccountData.Address.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())
	d.AssertCommittee(currentEpoch+2, append(d.AccountsFromNodes(d.Nodes("V1", "V3", "V2", "V4")...), goodAccountAddrBech32, lazyAccountAddrBech32))

	// issue validation blocks to have performance
	if currentSlot := clt.CommittedAPI().TimeProvider().CurrentSlot(); currentSlot < claimingSlot {
		slotToWait := claimingSlot - currentSlot
		secToWait := time.Duration(slotToWait) * time.Duration(slotsDuration) * time.Second
		fmt.Println("Wait for ", secToWait, "until expected slot: ", claimingSlot)

		var wg sync.WaitGroup
		issueValidationBlockInBackground(&wg, goodWallet, currentSlot, claimingSlot, 5)
		issueValidationBlockInBackground(&wg, lazyWallet, currentSlot, claimingSlot, 1)

		wg.Wait()
	}

	// claim rewards that put to the account output
	d.AwaitCommitment(claimingSlot)
	d.ClaimRewardsForValidator(ctx, goodWallet)
	d.ClaimRewardsForValidator(ctx, lazyWallet)

	// check if the mana increased as expected
	goodWalletAccountOutput := goodWallet.BlockIssuer.AccountData.Output
	require.Greater(t, goodWalletAccountOutput.StoredMana(), initialMana)

	lazyWalletAccountOutput := lazyWallet.BlockIssuer.AccountData.Output
	require.Greater(t, lazyWalletAccountOutput.StoredMana(), lazyInitialMana)

	// account that issued more validation blocks should have more mana
	require.Greater(t, goodWalletAccountOutput.StoredMana(), lazyWalletAccountOutput.StoredMana())
}

// Test_DelegatorRewards tests the rewards for a delegator.
// 1. Create an account and delegate funds to a validator.
// 2. Wait long enough so there's rewards can be claimed.
// 3. Claim rewards and check if the mana increased as expected.
func Test_DelegatorRewards(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 3),
			iotago.WithLivenessOptions(10, 10, 2, 4, 5),
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
	delegatorWallet, _ := d.CreateAccount()
	clt := delegatorWallet.Client

	// delegate funds to V2
	delegationOutputData := d.DelegateToValidator(delegatorWallet, d.Node("V2").AccountAddress(t))
	d.AwaitCommitment(delegationOutputData.ID.CreationSlot())

	// check if V2 received the delegator stake
	v2Resp, err := clt.Validator(ctx, d.Node("V2").AccountAddress(t))
	require.NoError(t, err)
	require.Greater(t, v2Resp.PoolStake, v2Resp.ValidatorStake)

	// wait until next epoch so the rewards can be claimed
	//nolint:forcetypeassert
	expectedSlot := clt.CommittedAPI().TimeProvider().EpochStart(delegationOutputData.Output.(*iotago.DelegationOutput).StartEpoch + 2)
	if currentSlot := clt.CommittedAPI().TimeProvider().CurrentSlot(); currentSlot < expectedSlot {
		slotToWait := expectedSlot - currentSlot
		secToWait := time.Duration(slotToWait) * time.Duration(clt.CommittedAPI().ProtocolParameters().SlotDurationInSeconds()) * time.Second
		fmt.Println("Wait for ", secToWait, "until expected slot: ", expectedSlot)
		time.Sleep(secToWait)
	}

	// claim rewards that put to an basic output
	rewardsOutputID := d.ClaimRewardsForDelegator(ctx, delegatorWallet, delegationOutputData)

	// check if the mana increased as expected
	outputFromAPI, err := clt.OutputByID(ctx, rewardsOutputID)
	require.NoError(t, err)

	rewardsOutput := delegatorWallet.Output(rewardsOutputID)
	require.Equal(t, rewardsOutput.Output.StoredMana(), outputFromAPI.StoredMana())
}

// Test_DelayedClaimingRewards tests the delayed claiming rewards for a delegator.
// 1. Create an account and delegate funds to a validator.
// 2. Delay claiming rewards for the delegation and check if the delegated stake is removed from the validator.
// 3. Claim rewards and check to destroy the delegation output.
func Test_DelayedClaimingRewards(t *testing.T) {
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
	delegatorWallet, _ := d.CreateAccount()
	clt := delegatorWallet.Client

	{
		// delegate funds to V2
		delegationOutputData := d.DelegateToValidator(delegatorWallet, d.Node("V2").AccountAddress(t))
		d.AwaitCommitment(delegationOutputData.ID.CreationSlot())

		// check if V2 received the delegator stake
		v2Resp, err := clt.Validator(ctx, d.Node("V2").AccountAddress(t))
		require.NoError(t, err)
		require.Greater(t, v2Resp.PoolStake, v2Resp.ValidatorStake)

		// delay claiming rewards
		currentSlot := delegatorWallet.CurrentSlot()
		apiForSlot := clt.APIForSlot(currentSlot)
		latestCommitmentSlot := delegatorWallet.GetNewBlockIssuanceResponse().LatestCommitment.Slot
		delegationEndEpoch := getDelegationEndEpoch(apiForSlot, currentSlot, latestCommitmentSlot)
		delegationOutputData = d.DelayedClaimingTransition(ctx, delegatorWallet, delegationOutputData)
		d.AwaitCommitment(delegationOutputData.ID.CreationSlot())

		// the delegated stake should be removed from the validator, so the pool stake should equal to the validator stake
		v2Resp, err = clt.Validator(ctx, d.Node("V2").AccountAddress(t))
		require.NoError(t, err)
		require.Equal(t, v2Resp.PoolStake, v2Resp.ValidatorStake)

		// wait until next epoch to destroy the delegation
		expectedSlot := clt.CommittedAPI().TimeProvider().EpochStart(delegationEndEpoch)
		if currentSlot := delegatorWallet.CurrentSlot(); currentSlot < expectedSlot {
			slotToWait := expectedSlot - currentSlot
			secToWait := time.Duration(slotToWait) * time.Duration(clt.CommittedAPI().ProtocolParameters().SlotDurationInSeconds()) * time.Second
			fmt.Println("Wait for ", secToWait, "until expected slot: ", expectedSlot)
			time.Sleep(secToWait)
		}
		fmt.Println("Claim rewards for delegator")
		d.ClaimRewardsForDelegator(ctx, delegatorWallet, delegationOutputData)
	}

	{
		// delegate funds to V2
		delegationOutputData := d.DelegateToValidator(delegatorWallet, d.Node("V2").AccountAddress(t))

		// delay claiming rewards in the same slot of delegation
		delegationOutputData = d.DelayedClaimingTransition(ctx, delegatorWallet, delegationOutputData)
		d.AwaitCommitment(delegationOutputData.ID.CreationSlot())

		// the delegated stake should be 0, thus poolStake should be equal to validatorStake
		v2Resp, err := clt.Validator(ctx, d.Node("V2").AccountAddress(t))
		require.NoError(t, err)
		require.Equal(t, v2Resp.PoolStake, v2Resp.ValidatorStake)

		// wait until next epoch to destroy the delegation
		d.ClaimRewardsForDelegator(ctx, delegatorWallet, delegationOutputData)
	}
}

func issueCandidacyPayloadInBackground(d *DockerTestFramework, wallet *mock.Wallet, startSlot, endSlot iotago.SlotIndex, slotDuration uint8) {
	go func() {
		fmt.Println("Issuing candidacy payloads for account", wallet.BlockIssuer.AccountData.ID, "in the background...")
		defer fmt.Println("Issuing candidacy payloads for account", wallet.BlockIssuer.AccountData.ID, "in the background......done")

		for i := startSlot; i < endSlot; i++ {
			d.IssueCandidacyPayloadFromAccount(wallet)
			time.Sleep(time.Duration(slotDuration) * time.Second)
		}
	}()
}

func issueValidationBlockInBackground(wg *sync.WaitGroup, wallet *mock.Wallet, startSlot, endSlot iotago.SlotIndex, blocksPerSlot int) {
	wg.Add(1)

	go func() {
		defer wg.Done()
		fmt.Println("Issuing validation block for wallet", wallet.Name, "in the background...")
		defer fmt.Println("Issuing validation block for wallet", wallet.Name, "in the background......done")

		for i := startSlot; i < endSlot; i++ {
			// wait until the slot is reached
			for {
				if wallet.CurrentSlot() == i {
					break
				}
				time.Sleep(2 * time.Second)
			}

			for range blocksPerSlot {
				wallet.CreateAndSubmitValidationBlock(context.Background(), "", nil)
				time.Sleep(1 * time.Second)
			}
		}
	}()
}
