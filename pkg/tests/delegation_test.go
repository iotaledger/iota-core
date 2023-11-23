package tests

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func setupDelegationTestsuite(t *testing.T) (*testsuite.TestSuite, *mock.Node, *mock.Node, []iotago.BlockID) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				8,
			),
			iotago.WithLivenessOptions(
				testsuite.DefaultLivenessThresholdLowerBoundInSeconds,
				testsuite.DefaultLivenessThresholdUpperBoundInSeconds,
				testsuite.DefaultMinCommittableAge,
				100,
				120,
			),
		),
	)

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	node2 := ts.AddNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet := ts.AddGenesisWallet("default", node1, iotago.MaxBlockIssuanceCredits/2)

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
		ValidatorStake:  mock.MinValidatorAccountAmount,
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

	// CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	// set the expiry slot of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

	var block1Slot iotago.SlotIndex = 1
	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX1",
		"Genesis:0",
		ts.DefaultWallet(),
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		mock.WithStakingFeature(10000, 421, 0, 10),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount),
	)

	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	ts.SetCurrentSlot(block1Slot)
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)
	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

	newAccount := ts.DefaultWallet().AccountOutput("TX1:0")
	newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountDiff(newAccountOutput.AccountID, block1Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedSlot:    0,
		NewExpirySlot:          newAccountExpirySlot,
		PreviousExpirySlot:     0,
		NewOutputID:            newAccount.OutputID(),
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   10000,
		StakeEndEpochChange:    10,
		FixedCostChange:        421,
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: 0,
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	return ts, node1, node2, latestParents
}

// Test that a Delegation Output which delegates to an account which does not exist / did not receive rewards
// can be destroyed.
func Test_Delegation_DestroyOutputWithoutRewards(t *testing.T) {
	ts, node1, node2, latestParents := setupDelegationTestsuite(t)
	defer ts.Shutdown()

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := tpkg.RandAccountAddress()
	block2Slot := ts.CurrentSlot()
	tx2 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX2",
		"TX1:1",
		mock.WithDelegatedValidatorAddress(accountAddress),
		mock.WithDelegationStartEpoch(1),
	)
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())

	block3Slot := ts.CurrentSlot()
	tx3 := ts.DefaultWallet().ClaimDelegatorRewards("TX3", "TX2:0")
	block3 := ts.IssueBasicBlockWithOptions("block3", ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParents...))

	ts.CommitUntilSlot(block3Slot, block3.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx3.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx3.Transaction}, true, node1, node2)
}

func Test_RewardInputCannotPointToNFTOutput(t *testing.T) {
	ts, node1, node2, latestParents := setupDelegationTestsuite(t)
	defer ts.Shutdown()

	// CREATE NFT FROM BASIC UTXO
	block2Slot := ts.CurrentSlot()
	input := ts.DefaultWallet().Output("TX1:1")
	nftOutput := builder.NewNFTOutputBuilder(ts.DefaultWallet().Address(), input.BaseTokenAmount()).MustBuild()
	tx2 := ts.DefaultWallet().CreateSignedTransactionWithOptions(
		"TX2",
		mock.WithInputs(utxoledger.Outputs{input}),
		mock.WithOutputs(iotago.Outputs[iotago.Output]{nftOutput}),
		mock.WithAllotAllManaToAccount(ts.CurrentSlot(), ts.DefaultWallet().BlockIssuer.AccountID),
	)

	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())

	ts.AssertTransactionsExist([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx2.Transaction}, true, node1, node2)

	// ATTEMPT TO POINT REWARD INPUT TO AN NFT OUTPUT
	inputNFT := ts.DefaultWallet().Output("TX2:0")
	prevNFT := inputNFT.Output().Clone().(*iotago.NFTOutput)
	nftOutput = builder.NewNFTOutputBuilderFromPrevious(prevNFT).NFTID(iotago.NFTIDFromOutputID(inputNFT.OutputID())).MustBuild()

	tx3 := ts.DefaultWallet().CreateSignedTransactionWithOptions(
		"TX3",
		mock.WithInputs(utxoledger.Outputs{inputNFT}),
		mock.WithRewardInput(
			&iotago.RewardInput{Index: 0},
			0,
		),
		mock.WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: ts.DefaultWallet().Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		mock.WithOutputs(iotago.Outputs[iotago.Output]{nftOutput}),
		mock.WithAllotAllManaToAccount(ts.CurrentSlot(), ts.DefaultWallet().BlockIssuer.AccountID),
	)

	block3 := ts.IssueBasicBlockWithOptions("block3", ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParents...))

	ts.Wait(node1, node2)

	ts.AssertTransactionsExist([]*iotago.Transaction{tx3.Transaction}, true, node1)
	ts.AssertTransactionFailureReason(block3.ID(), apimodels.TxFailureRewardInputInvalid, node1)
}
