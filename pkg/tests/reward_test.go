package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func setupRewardTestsuite(t *testing.T) (*testsuite.TestSuite, *mock.Node, *mock.Node) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				8,
			),
			iotago.WithStakingOptions(2, 10, 10),
		),
	)

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	node2 := ts.AddNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true)

	// Assert validator and block issuer accounts in genesis snapshot.
	// Validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountData.ID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.ID,
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
	}, ts.Nodes()...)
	// Default wallet block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountData.ID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.ID,
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	return ts, node1, node2
}

// Test that a Delegation Output which delegates to an account which does not exist / did not receive rewards
// can be destroyed.
func Test_Delegation_DestroyOutputWithoutRewards(t *testing.T) {
	ts, node1, node2 := setupRewardTestsuite(t)
	defer ts.Shutdown()

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := tpkg.RandAccountAddress()
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)
	tx1 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX1",
		ts.DefaultWallet().OutputData("Genesis:0"),
		mock.WithDelegatedValidatorAddress(accountAddress),
		mock.WithDelegationStartEpoch(1),
	)
	block1 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1))

	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	block2Slot := ts.CurrentSlot()
	tx2 := ts.DefaultWallet().ClaimDelegatorRewards("TX2", "TX1:0")
	block2 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...)))

	ts.CommitUntilSlot(block2Slot, block2.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)
}

func Test_Delegation_DelayedClaimingDestroyOutputWithoutRewards(t *testing.T) {
	ts, node1, node2 := setupRewardTestsuite(t)
	defer ts.Shutdown()

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := tpkg.RandAccountAddress()
	var block1_2Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1_2Slot)
	tx1 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX1",
		ts.DefaultWallet().OutputData("Genesis:0"),
		mock.WithDelegatedValidatorAddress(accountAddress),
		mock.WithDelegationStartEpoch(1),
	)
	block1 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1))

	// TRANSITION TO DELAYED CLAIMING (IN THE SAME SLOT)
	blockIssuanceInfo, err := ts.DefaultWallet().Client.BlockIssuance(context.TODO())
	require.NoError(t, err)
	latestCommitment := blockIssuanceInfo.LatestCommitment
	apiForSlot := ts.DefaultWallet().Client.APIForSlot(block1_2Slot)

	futureBoundedSlotIndex := latestCommitment.Slot + apiForSlot.ProtocolParameters().MinCommittableAge()
	futureBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	registrationSlot := apiForSlot.TimeProvider().EpochEnd(apiForSlot.TimeProvider().EpochFromSlot(block1_2Slot))
	delegationEndEpoch := futureBoundedEpochIndex
	if futureBoundedSlotIndex > registrationSlot {
		delegationEndEpoch = futureBoundedEpochIndex + 1
	}

	tx2 := ts.DefaultWallet().DelayedClaimingTransition("TX2", "TX1:0", delegationEndEpoch)
	block2 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(block1.ID())))
	latestParents := ts.CommitUntilSlot(block1_2Slot, block2.ID())

	// CLAIM ZERO REWARDS
	block3Slot := ts.CurrentSlot()
	tx3 := ts.DefaultWallet().ClaimDelegatorRewards("TX3", "TX2:0")
	block3 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block3", ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParents...)))

	ts.CommitUntilSlot(block3Slot, block3.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx3.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx3.Transaction}, true, node1, node2)
}

// Test that a staking Account which did not earn rewards can remove its staking feature.
func Test_Account_RemoveStakingFeatureWithoutRewards(t *testing.T) {
	ts, node1, node2 := setupRewardTestsuite(t)
	defer ts.Shutdown()

	// CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)

	// Set end epoch so the staking feature can be removed as soon as possible.
	unbondingPeriod := ts.API.ProtocolParameters().StakingUnbondingPeriod()
	startEpoch := ts.API.TimeProvider().EpochFromSlot(block1Slot + ts.API.ProtocolParameters().MaxCommittableAge())
	endEpoch := startEpoch + unbondingPeriod
	// The earliest epoch in which we can remove the staking feature and claim rewards.
	claimingEpoch := endEpoch + 1
	// Random fixed cost amount.
	fixedCost := iotago.Mana(421)
	stakedAmount := mock.MinValidatorAccountAmount(ts.API.ProtocolParameters())

	blockIssuerFeatKey := tpkg.RandBlockIssuerKey()
	// Set the expiry slot beyond the end epoch of the staking feature so we don't have to remove the feature.
	blockIssuerFeatExpirySlot := ts.API.TimeProvider().EpochEnd(claimingEpoch)

	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX1",
		"Genesis:0",
		ts.DefaultWallet(),
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{blockIssuerFeatKey}, blockIssuerFeatExpirySlot),
		mock.WithStakingFeature(stakedAmount, fixedCost, startEpoch, endEpoch),
		mock.WithAccountAmount(stakedAmount),
	)

	block1 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1))

	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx1.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx1.Transaction}, true, node1, node2)

	// Commit until the claiming epoch.
	latestParents = ts.CommitUntilSlot(ts.API.TimeProvider().EpochStart(claimingEpoch), latestParents...)

	// REMOVE STAKING FEATURE AND CLAIM ZERO REWARDS
	block2Slot := ts.CurrentSlot()
	tx2 := ts.DefaultWallet().ClaimValidatorRewards("TX2", "TX1:0")
	block2 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...)))

	ts.CommitUntilSlot(block2Slot, block2.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx2.Transaction}, true, node1)
	accountOutput := ts.DefaultWallet().OutputData("TX2:0")
	accountID := accountOutput.Output.(*iotago.AccountOutput).AccountID

	ts.AssertAccountData(&accounts.AccountData{
		ID:              accountID,
		Credits:         &accounts.BlockIssuanceCredits{Value: 0, UpdateSlot: block1Slot},
		OutputID:        accountOutput.ID,
		ExpirySlot:      blockIssuerFeatExpirySlot,
		BlockIssuerKeys: iotago.BlockIssuerKeys{blockIssuerFeatKey},
		StakeEndEpoch:   0,
		ValidatorStake:  0,
	}, ts.Nodes()...)

	ts.AssertAccountDiff(accountID, block2Slot, &model.AccountDiff{
		BICChange:              -iotago.BlockIssuanceCredits(0),
		PreviousUpdatedSlot:    0,
		NewExpirySlot:          blockIssuerFeatExpirySlot,
		PreviousExpirySlot:     blockIssuerFeatExpirySlot,
		NewOutputID:            accountOutput.ID,
		PreviousOutputID:       ts.DefaultWallet().OutputData("TX1:0").ID,
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   -int64(stakedAmount),
		StakeEndEpochChange:    -int64(endEpoch),
		FixedCostChange:        -int64(fixedCost),
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)
}

func Test_RewardInputCannotPointToNFTOutput(t *testing.T) {
	ts, node1, node2 := setupRewardTestsuite(t)
	defer ts.Shutdown()

	// CREATE NFT FROM BASIC UTXO
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)

	tx1 := ts.DefaultWallet().CreateNFTFromInput("TX1", ts.DefaultWallet().OutputData("Genesis:0"))
	block1 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1))

	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx1.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx1.Transaction}, true, node1, node2)

	// ATTEMPT TO POINT REWARD INPUT TO AN NFT OUTPUT
	tx2 := ts.DefaultWallet().TransitionNFTWithTransactionOpts("TX2", "TX1:0",
		mock.WithRewardInput(
			&iotago.RewardInput{Index: 0},
			0,
		),
		mock.WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: ts.DefaultWallet().GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}))

	ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	ts.Wait(node1, node2)

	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)
	signedTx2ID := tx2.MustID()
	ts.AssertTransactionFailure(signedTx2ID, iotago.ErrRewardInputReferenceInvalid, node1, node2)
}

// Test that delegations in all forms are correctly reflected in the staked and delegated amounts.
func Test_Account_StakeAmountCalculation(t *testing.T) {
	ts, _, _ := setupRewardTestsuite(t)
	defer ts.Shutdown()

	// STEP 1: CREATE NEW ACCOUNT WITH A BLOCK ISSUER FEATURE FROM BASIC UTXO.
	// This account is not a staker yet.
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)

	// Set end epoch so the staking feature can be removed as soon as possible.
	unbondingPeriod := ts.API.ProtocolParameters().StakingUnbondingPeriod()
	startEpoch := ts.API.TimeProvider().EpochFromSlot(block1Slot + ts.API.ProtocolParameters().MaxCommittableAge())
	endEpoch := startEpoch + unbondingPeriod
	// The earliest epoch in which we can remove the staking feature and claim rewards.
	claimingEpoch := endEpoch + 1
	// Random fixed cost amount.
	fixedCost := iotago.Mana(421)
	stakedAmount := mock.MinValidatorAccountAmount(ts.API.ProtocolParameters())

	blockIssuerFeatKey := tpkg.RandBlockIssuerKey()
	// Set the expiry slot beyond the end epoch of the staking feature so we don't have to remove the feature.
	blockIssuerFeatExpirySlot := ts.API.TimeProvider().EpochEnd(claimingEpoch)

	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX1",
		"Genesis:0",
		ts.DefaultWallet(),
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{blockIssuerFeatKey}, blockIssuerFeatExpirySlot),
		mock.WithAccountAmount(stakedAmount),
	)

	block1 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1))
	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	account := ts.DefaultWallet().OutputData("TX1:0")
	accountID := iotago.AccountIDFromOutputID(account.ID)
	accountAddress := iotago.AccountAddress(accountID[:])

	ts.AssertAccountStake(accountID, 0, 0, ts.Nodes()...)

	// STEP 2: CREATE DELEGATION OUTPUT DELEGATING TO THE ACCOUNT.
	block2Slot := ts.CurrentSlot()
	deleg1 := mock.MinDelegationAmount(ts.API.ProtocolParameters())
	blockIssuanceResp := ts.DefaultWallet().GetNewBlockIssuanceResponse()
	tx2 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX2",
		ts.DefaultWallet().OutputData("TX1:1"),
		mock.WithDelegationAmount(deleg1),
		mock.WithDelegatedAmount(deleg1),
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(ts.DefaultWallet().DelegationStartFromSlot(block2Slot, blockIssuanceResp.LatestCommitment.Slot)),
	)

	block2 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...)))
	latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())

	ts.AssertAccountStake(accountID, 0, deleg1, ts.Nodes()...)

	// STEP 3: TURN ACCOUNT INTO A REGISTERED VALIDATOR AND CREATE ANOTHER DELEGATION.

	block3_4Slot := ts.CurrentSlot()
	tx3 := ts.DefaultWallet().TransitionAccount("TX3", "TX1:0",
		mock.WithStakingFeature(stakedAmount, fixedCost, startEpoch, endEpoch),
	)
	block3 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block3", ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParents...)))

	deleg2 := mock.MinDelegationAmount(ts.API.ProtocolParameters()) + 200
	blockIssuanceResp = ts.DefaultWallet().GetNewBlockIssuanceResponse()
	// Create another delegation.
	tx4 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX4",
		ts.DefaultWallet().OutputData("TX2:1"),
		mock.WithDelegationAmount(deleg2),
		mock.WithDelegatedAmount(deleg2),
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(ts.DefaultWallet().DelegationStartFromSlot(block2Slot, blockIssuanceResp.LatestCommitment.Slot)),
	)
	block4 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block4", ts.DefaultWallet(), tx4, mock.WithStrongParents(block3.ID())))
	latestParents = ts.CommitUntilSlot(block3_4Slot, block4.ID())

	ts.AssertAccountStake(accountID, stakedAmount, deleg1+deleg2, ts.Nodes()...)

	// STEP 4: CREATE A DELEGATION TO THE ACCOUNT AND TRANSITION IT TO DELAYED CLAIMING IN THE SAME SLOT.
	block5_6Slot := ts.CurrentSlot()
	deleg3 := mock.MinDelegationAmount(ts.API.ProtocolParameters()) + 352
	blockIssuanceResp = ts.DefaultWallet().GetNewBlockIssuanceResponse()
	tx5 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX5",
		ts.DefaultWallet().OutputData("TX4:1"),
		mock.WithDelegationAmount(deleg3),
		mock.WithDelegatedAmount(deleg3),
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(ts.DefaultWallet().DelegationStartFromSlot(block5_6Slot, blockIssuanceResp.LatestCommitment.Slot)),
	)
	block5 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block5", ts.DefaultWallet(), tx5, mock.WithStrongParents(latestParents...)))
	blockIssuanceResp = ts.DefaultWallet().GetNewBlockIssuanceResponse()
	tx6 := ts.DefaultWallet().DelayedClaimingTransition("TX6", "TX5:0", ts.DefaultWallet().DelegationEndFromSlot(block5_6Slot, blockIssuanceResp.LatestCommitment.Slot))
	block6 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block6", ts.DefaultWallet(), tx6, mock.WithStrongParents(block5.ID())))

	latestParents = ts.CommitUntilSlot(block5_6Slot, block6.ID())

	// Delegated Stake should be unaffected since delayed claiming delegations do not count.
	ts.AssertAccountStake(accountID, stakedAmount, deleg1+deleg2, ts.Nodes()...)

	// STEP 5: CREATE A DELEGATION TO THE ACCOUNT AND DESTROY IT IN THE SAME SLOT.
	block7_8Slot := ts.CurrentSlot()
	deleg4 := mock.MinDelegationAmount(ts.API.ProtocolParameters()) + 153
	blockIssuanceResp = ts.DefaultWallet().GetNewBlockIssuanceResponse()
	tx7 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX7",
		ts.DefaultWallet().OutputData("TX5:1"),
		mock.WithDelegationAmount(deleg4),
		mock.WithDelegatedAmount(deleg4),
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(ts.DefaultWallet().DelegationStartFromSlot(block7_8Slot, blockIssuanceResp.LatestCommitment.Slot)),
	)
	block7 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block7", ts.DefaultWallet(), tx7, mock.WithStrongParents(latestParents...)))

	latestParents = ts.CommitUntilSlot(block7_8Slot, block7.ID())

	tx8 := ts.DefaultWallet().ClaimDelegatorRewards("TX8", "TX7:0")
	block8 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block8", ts.DefaultWallet(), tx8, mock.WithStrongParents(latestParents...)))

	block8Slot := ts.CurrentSlot()
	latestParents = ts.CommitUntilSlot(block8Slot, block8.ID())

	// Delegated Stake should be unaffected since no new delegation was effectively added in that slot.
	ts.AssertAccountStake(accountID, stakedAmount, deleg1+deleg2, ts.Nodes()...)

	// STEP 6: REMOVE A DELEGATION BY TRANSITIONING TO DELAYED CLAIMING.
	block9Slot := ts.CurrentSlot()
	blockIssuanceResp = ts.DefaultWallet().GetNewBlockIssuanceResponse()
	tx9 := ts.DefaultWallet().DelayedClaimingTransition("TX9", "TX4:0", ts.DefaultWallet().DelegationEndFromSlot(block9Slot, blockIssuanceResp.LatestCommitment.Slot))
	block9 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block9", ts.DefaultWallet(), tx9, mock.WithStrongParents(latestParents...)))
	// Commit until the claiming epoch so we can remove the staking feature from the account in the next step.
	latestParents = ts.CommitUntilSlot(block9Slot, block9.ID())

	ts.AssertAccountStake(accountID, stakedAmount, deleg1, ts.Nodes()...)

	// STEP 7: DESTROY THE DELEGATION IN DELAYED CLAIMING STATE
	// This is to ensure the delegated stake is not subtracted twice from the account.
	tx10 := ts.DefaultWallet().ClaimDelegatorRewards("TX19", "TX9:0")
	block10 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block10", ts.DefaultWallet(), tx10, mock.WithStrongParents(latestParents...)))

	// Commit until the claiming epoch so we can remove the staking feature from the account in the next step.
	latestParents = ts.CommitUntilSlot(ts.API.TimeProvider().EpochStart(claimingEpoch), block10.ID())

	ts.AssertAccountStake(accountID, stakedAmount, deleg1, ts.Nodes()...)

	// STEP 8: DESTROY ACCOUNT.
	block11Slot := ts.CurrentSlot()
	tx11 := ts.DefaultWallet().ClaimValidatorRewards("TX11", "TX3:0")
	block11 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block11", ts.DefaultWallet(), tx11, mock.WithStrongParents(latestParents...)))
	latestParents = ts.CommitUntilSlot(block11Slot, block11.ID())

	ts.AssertAccountStake(accountID, 0, deleg1, ts.Nodes()...)

	// STEP 9: TRANSITION INITIAL DELEGATION TO DELAYED CLAIMING.
	// Ensure that the accounts ledger is correctly updated in this edge case
	// where the account to which it points no longer exists.

	block12Slot := ts.CurrentSlot()
	blockIssuanceResp = ts.DefaultWallet().GetNewBlockIssuanceResponse()
	tx12 := ts.DefaultWallet().DelayedClaimingTransition("TX12", "TX2:0", ts.DefaultWallet().DelegationEndFromSlot(block12Slot, blockIssuanceResp.LatestCommitment.Slot))
	block12 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block12", ts.DefaultWallet(), tx12, mock.WithStrongParents(latestParents...)))
	ts.CommitUntilSlot(block12Slot, block12.ID())

	ts.AssertAccountStake(accountID, 0, 0, ts.Nodes()...)
}
