package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: implement tests for staking and delegation transitions that cover edge cases - part of hardening phase.
func Test_TransitionAccount(t *testing.T) {
	oldGenesisOutputKey := utils.RandBlockIssuerKey()
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		// Nil address will be replaced with the address generated from genesis seed.
		// A single key may unlock multiple accounts; that's why it can't be used as a source for AccountID derivation.
		Address: nil,
		// Set an amount enough to cover the rent and to cover an additional key that is added in the test.
		// If it's too little, then the test will fail.
		Amount: testsuite.MinIssuerAccountAmount * 10,
		Mana:   0,
		// AccountID is derived from this field, so this must be set uniquely for each account.
		IssuerKey: oldGenesisOutputKey,
		// Expiry Slot is the slot index at which the account expires.
		ExpirySlot: 1,
		// BlockIssuanceCredits on this account is custom because it never needs to issue.
		// On Validator nodes it's unlimited (MaxInt64).
		BlockIssuanceCredits: iotago.BlockIssuanceCredits(123),
	}),
		testsuite.WithGenesisTimestampOffset(100*10),
		testsuite.WithMaxCommittableAge(100),
		testsuite.WithSlotsPerEpochExponent(8),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	_ = ts.AddNode("node2")

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	genesisAccount := ts.AccountOutput("Genesis:1")
	genesisAccountOutput := genesisAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        genesisAccount.OutputID(),
		ExpirySlot:      1,
		BlockIssuerKeys: ds.NewSet(oldGenesisOutputKey),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT AND PREPARE SOME BASIC OUTPUTS

	newGenesisOutputKey := utils.RandBlockIssuerKey()
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	latestCommitmentID := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID()

	accountInput, accountOutputs, accountWallets := ts.TransactionFramework.TransitionAccount(
		"Genesis:1",
		testsuite.AddBlockIssuerKey(newGenesisOutputKey),
		testsuite.WithBlockIssuerExpirySlot(1),
	)
	consumedInputs, equalOutputs, equalWallets := ts.TransactionFramework.CreateBasicOutputsEqually(5, "Genesis:0")

	tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX1", append(accountWallets, equalWallets...),
		testsuite.WithAccountInput(accountInput, true),
		testsuite.WithInputs(consumedInputs),
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: genesisAccountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: latestCommitmentID,
			},
		}),
		testsuite.WithOutputs(append(accountOutputs, equalOutputs...)),
		testsuite.WithAllotments(iotago.Allotments{&iotago.Allotment{
			AccountID: genesisAccountOutput.AccountID,
			Value:     0,
		}}),
	))

	var slotIndexBlock1 iotago.SlotIndex = 1
	activeNodes := []*mock.Node{node1}
	genesisCommitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost

	block1 := ts.IssueBlockAtSlotWithOptions("block1", slotIndexBlock1, genesisCommitment, node1, tx1)

	latestParent := ts.CommitUntilSlot(ts.BlockID("block1").Index(), activeNodes, block1)

	ts.AssertAccountDiff(genesisAccountOutput.AccountID, slotIndexBlock1, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		PreviousExpirySlot:     1,
		NewExpirySlot:          1,
		NewOutputID:            iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID(ts.API)), 0),
		PreviousOutputID:       genesisAccount.OutputID(),
		BlockIssuerKeysRemoved: iotago.BlockIssuerKeys{},
		BlockIssuerKeysAdded:   iotago.BlockIssuerKeys{newGenesisOutputKey},
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID(ts.API)), 0),
		BlockIssuerKeys: ds.NewSet(oldGenesisOutputKey, newGenesisOutputKey),
		ExpirySlot:      1,
	}, ts.Nodes()...)

	// DESTROY GENESIS ACCOUNT, CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO

	// commit until the expiry slot of the transitioned genesis account plus one
	latestParent = ts.CommitUntilSlot(accountOutputs[0].FeatureSet().BlockIssuer().ExpirySlot+1, activeNodes, latestParent)
	// set the expiry slof of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index() + ts.API.ProtocolParameters().MaxCommittableAge()
	inputForNewAccount, newAccountOutputs, newAccountWallets := ts.TransactionFramework.CreateAccountFromInput("TX1:1",
		testsuite.WithAccountConditions(iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
			&iotago.GovernorAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
		}),
		testsuite.WithBlockIssuerFeature(&iotago.BlockIssuerFeature{
			BlockIssuerKeys: iotago.BlockIssuerKeys{newAccountBlockIssuerKey},
			ExpirySlot:      newAccountExpirySlot,
		}),
		testsuite.WithStakingFeature(&iotago.StakingFeature{
			StakedAmount: 10000,
			FixedCost:    421,
			StartEpoch:   0,
			EndEpoch:     10,
		}),
	)

	destroyedAccountInput, destructionOutputs, destroyWallets := ts.TransactionFramework.DestroyAccount("TX1:0")

	slotIndexBlock2 := latestParent.ID().Index()

	tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX2", append(newAccountWallets, destroyWallets...),
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: genesisAccountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForNewAccount),
		testsuite.WithAccountInput(destroyedAccountInput, true),
		testsuite.WithOutputs(append(newAccountOutputs, destructionOutputs...)),
		testsuite.WithCreationSlot(slotIndexBlock2),
	))

	block2 := ts.IssueBlockAtSlotWithOptions("block2", slotIndexBlock2, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, tx2, blockfactory.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(slotIndexBlock2, activeNodes, block2)

	// assert diff of a destroyed account, to make sure we can correctly restore it
	ts.AssertAccountDiff(genesisAccountOutput.AccountID, slotIndexBlock2, &model.AccountDiff{
		BICChange:              -iotago.BlockIssuanceCredits(123),
		PreviousUpdatedTime:    0,
		NewExpirySlot:          0,
		PreviousExpirySlot:     1,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID(ts.API)), 0),
		BlockIssuerKeysAdded:   iotago.BlockIssuerKeys{},
		BlockIssuerKeysRemoved: iotago.BlockIssuerKeys{oldGenesisOutputKey, newGenesisOutputKey},
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  0,
	}, true, ts.Nodes()...)

	newAccount := ts.AccountOutput("TX2:0")
	newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountDiff(newAccountOutput.AccountID, slotIndexBlock2, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewExpirySlot:          newAccountExpirySlot,
		PreviousExpirySlot:     0,
		NewOutputID:            newAccount.OutputID(),
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.BlockIssuerKeys{newAccountBlockIssuerKey},
		BlockIssuerKeysRemoved: iotago.BlockIssuerKeys{},
		ValidatorStakeChange:   10000,
		StakeEndEpochChange:    10,
		FixedCostChange:        421,
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, slotIndexBlock2),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: ds.NewSet(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: 0,
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	// create a delegation output delegating to the newly created account
	inputForNewDelegation, newDelegationOutputs, newDelegationWallets := ts.TransactionFramework.CreateDelegationFromInput("TX1:2",
		testsuite.WithDelegatedValidatorID(newAccountOutput.AccountID),
		testsuite.WithDelegationStartEpoch(1),
	)

	slotIndexBlock3 := latestParent.ID().Index()

	tx3 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX3", newDelegationWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForNewDelegation),
		testsuite.WithOutputs(newDelegationOutputs),
		testsuite.WithCreationSlot(slotIndexBlock3),
	))

	block3 := ts.IssueBlockAtSlotWithOptions("block3", slotIndexBlock3, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, tx3, blockfactory.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(slotIndexBlock3, activeNodes, block3)
	delegatedAmount := inputForNewDelegation[0].BaseTokenAmount()

	ts.AssertAccountDiff(newAccountOutput.AccountID, slotIndexBlock3, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.BlockIssuerKeys{},
		BlockIssuerKeysRemoved: iotago.BlockIssuerKeys{},
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  int64(delegatedAmount),
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, slotIndexBlock2),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: ds.NewSet(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: iotago.BaseToken(delegatedAmount),
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	// transition a delegation output to a delayed claiming state
	inputForDelegationTransition, delegationTransitionOutputs, delegationTransitionWallets := ts.TransactionFramework.DelayedClaimingTransition("TX3:0", 0)

	tx4 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX4", delegationTransitionWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForDelegationTransition),
		testsuite.WithOutputs(delegationTransitionOutputs),
		testsuite.WithCreationSlot(slotIndexBlock3),
	))

	slotIndexBlock4 := latestParent.ID().Index()

	block4 := ts.IssueBlockAtSlotWithOptions("block4", slotIndexBlock4, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, tx4, blockfactory.WithStrongParents(latestParent.ID()))

	_ = ts.CommitUntilSlot(slotIndexBlock4, activeNodes, block4)

	ts.AssertAccountDiff(newAccountOutput.AccountID, slotIndexBlock4, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.BlockIssuerKeys{},
		BlockIssuerKeysRemoved: iotago.BlockIssuerKeys{},
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, slotIndexBlock2),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: ds.NewSet(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: iotago.BaseToken(delegatedAmount),
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	ts.Wait(ts.Nodes()...)
}

/*
For Mana allotment and stored:
1. Collect potential and stored on the input side.
2. Add options to allot amounts to accounts upon TX creation.
3. Add option to store mana on the output side.
4. Optionally add option to split amount on outputs unevenly.

WithAllotments
{
	A1: amount
	A3: amount
}
WithStoredOnOutput
{
	0: amount
	3: amount
}
*/

/*
TX involving Accounts:
1. Add option to add accounts as inputs.
2. Add option to add accounts as outputs.
3. Create account.
4. Destroy accounts.
5. Accounts w/out and w/ BIC.

Testcases:
1. Create account w/out BIC from normal UTXO.
2. Create account w/ BIC from normal UTXO.
3. Transition non-BIC account to BIC account.
4. Transition BIC account to non-BIC account.
5. Transition BIC account to BIC account changing amount/keys/expiry.
6. Destroy account w/out BIC feature.
7. Destroy account w/ BIC feature.
*/
