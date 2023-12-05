package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func setupDelegationTestsuite(t *testing.T) (*testsuite.TestSuite, *mock.Node, *mock.Node) {
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
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
	}, ts.Nodes()...)
	// Default wallet block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	return ts, node1, node2
}

// Test that a Delegation Output which delegates to an account which does not exist / did not receive rewards
// can be destroyed.
func Test_Delegation_DestroyOutputWithoutRewards(t *testing.T) {
	ts, node1, node2 := setupDelegationTestsuite(t)
	defer ts.Shutdown()

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := tpkg.RandAccountAddress()
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)
	tx1 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX1",
		"Genesis:0",
		mock.WithDelegatedValidatorAddress(accountAddress),
		mock.WithDelegationStartEpoch(1),
	)
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)

	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	block2Slot := ts.CurrentSlot()
	tx2 := ts.DefaultWallet().ClaimDelegatorRewards("TX2", "TX1:0")
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	ts.CommitUntilSlot(block2Slot, block2.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)
}

func Test_Delegation_DelayedClaimingDestroyOutputWithoutRewards(t *testing.T) {
	ts, node1, node2 := setupDelegationTestsuite(t)
	defer ts.Shutdown()

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := tpkg.RandAccountAddress()
	var block1_2Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1_2Slot)
	tx1 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX1",
		"Genesis:0",
		mock.WithDelegatedValidatorAddress(accountAddress),
		mock.WithDelegationStartEpoch(1),
	)
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)

	// TRANSITION TO DELAYED CLAIMING (IN THE SAME SLOT)
	latestCommitment := ts.DefaultWallet().Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()
	apiForSlot := ts.DefaultWallet().Node.Protocol.APIForSlot(block1_2Slot)

	futureBoundedSlotIndex := latestCommitment.Slot() + apiForSlot.ProtocolParameters().MinCommittableAge()
	futureBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	registrationSlot := apiForSlot.TimeProvider().EpochEnd(apiForSlot.TimeProvider().EpochFromSlot(block1_2Slot))
	delegationEndEpoch := futureBoundedEpochIndex
	if futureBoundedSlotIndex > registrationSlot {
		delegationEndEpoch = futureBoundedEpochIndex + 1
	}

	tx2 := ts.DefaultWallet().DelayedClaimingTransition("TX2", "TX1:0", delegationEndEpoch)
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(block1.ID()))
	latestParents := ts.CommitUntilSlot(block1_2Slot, block2.ID())

	// CLAIM ZERO REWARDS
	block3Slot := ts.CurrentSlot()
	tx3 := ts.DefaultWallet().ClaimDelegatorRewards("TX3", "TX2:0")
	block3 := ts.IssueBasicBlockWithOptions("block3", ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParents...))

	ts.CommitUntilSlot(block3Slot, block3.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx3.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx3.Transaction}, true, node1, node2)
}

// Test that a staking Account which did not earn rewards can remove its staking feature.
func Test_Account_RemoveStakingFeatureWithoutRewards(t *testing.T) {
	ts, node1, node2 := setupDelegationTestsuite(t)
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

	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)

	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx1.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx1.Transaction}, true, node1, node2)

	// Commit until the claiming epoch.
	latestParents = ts.CommitUntilSlot(ts.API.TimeProvider().EpochStart(claimingEpoch), latestParents...)

	// REMOVE STAKING FEATURE AND CLAIM ZERO REWARDS
	block2Slot := ts.CurrentSlot()
	tx2 := ts.DefaultWallet().ClaimValidatorRewards("TX2", "TX1:0")
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	ts.CommitUntilSlot(block2Slot, block2.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx2.Transaction}, true, node1)
	accountOutput := ts.DefaultWallet().Output("TX2:0")
	accountID := accountOutput.Output().(*iotago.AccountOutput).AccountID

	ts.AssertAccountData(&accounts.AccountData{
		ID:              accountID,
		Credits:         &accounts.BlockIssuanceCredits{Value: 0, UpdateSlot: block1Slot},
		OutputID:        accountOutput.OutputID(),
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
		NewOutputID:            accountOutput.OutputID(),
		PreviousOutputID:       ts.DefaultWallet().Output("TX1:0").OutputID(),
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   -int64(stakedAmount),
		StakeEndEpochChange:    -int64(endEpoch),
		FixedCostChange:        -int64(fixedCost),
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)
}

func Test_RewardInputCannotPointToNFTOutput(t *testing.T) {
	ts, node1, node2 := setupDelegationTestsuite(t)
	defer ts.Shutdown()

	// CREATE NFT FROM BASIC UTXO
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)

	tx1 := ts.DefaultWallet().CreateNFTFromInput("TX1", "Genesis:0")
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)

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
			CommitmentID: ts.DefaultWallet().Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}))

	ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	ts.Wait(node1, node2)

	// TODO: Assertions do not pass for node2 because the block does not get forwarded from node1.
	// node2 should be added in the assertion when issue iotaledger/iota-core#580 is fixed.
	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1)
	signedTx2ID := lo.PanicOnErr(tx2.ID())
	ts.AssertTransactionFailure(signedTx2ID, iotago.ErrRewardInputInvalid, node1)
}
