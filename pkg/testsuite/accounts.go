package testsuite

import (
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertAccountData(accountData *accounts.AccountData, nodes ...*mock.Node) {
	t.Eventually(func() error {
		for _, node := range nodes {
			actualAccountData, exists, err := node.Protocol.MainEngine.Get().Ledger.Account(accountData.ID, node.Protocol.MainEngine.Get().SyncManager.LatestCommitment().Slot())
			if err != nil {
				return ierrors.Wrap(err, "AssertAccountData: failed to load account data")
			}
			if !exists {
				return ierrors.Errorf("AssertAccountData: %s: account %s does not exist with latest committed slot %d", node.Name, accountData.ID, node.Protocol.MainEngine.Get().SyncManager.LatestCommitment().Slot())
			}

			if accountData.ID != actualAccountData.ID {
				return ierrors.Errorf("AssertAccountData: %s: expected %s, got %s", node.Name, accountData.ID, actualAccountData.ID)
			}

			if accountData.Credits.Value != actualAccountData.Credits.Value {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected credits value %d, got %d", node.Name, accountData.ID, accountData.Credits.Value, actualAccountData.Credits.Value)
			}

			if accountData.Credits.UpdateTime != actualAccountData.Credits.UpdateTime {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected credits update time %d, got %d", node.Name, accountData.ID, accountData.Credits.UpdateTime, actualAccountData.Credits.UpdateTime)
			}

			if accountData.OutputID != actualAccountData.OutputID {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected output %s, got %s", node.Name, accountData.ID, accountData.OutputID, actualAccountData.OutputID)
			}

			if accountData.ExpirySlot != actualAccountData.ExpirySlot {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected expiry slot %s, got %s", node.Name, accountData.ID, accountData.ExpirySlot, actualAccountData.ExpirySlot)
			}

			if !assert.Equal(t.fakeTesting, accountData.BlockIssuerKeys, actualAccountData.BlockIssuerKeys) {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected pub keys %s, got %s", node.Name, accountData.ID, accountData.BlockIssuerKeys, actualAccountData.BlockIssuerKeys)
			}

			if accountData.StakeEndEpoch != actualAccountData.StakeEndEpoch {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected stake end epoch %s, got %s", node.Name, accountData.ID, accountData.StakeEndEpoch, actualAccountData.StakeEndEpoch)
			}

			if accountData.FixedCost != actualAccountData.FixedCost {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected fixed cost %d, got %d", node.Name, accountData.ID, accountData.FixedCost, actualAccountData.FixedCost)
			}

			if accountData.ValidatorStake != actualAccountData.ValidatorStake {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected validator stake %d, got %d", node.Name, accountData.ID, accountData.ValidatorStake, actualAccountData.ValidatorStake)
			}

			if accountData.DelegationStake != actualAccountData.DelegationStake {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected delegation stake %d, got %d", node.Name, accountData.ID, accountData.DelegationStake, actualAccountData.DelegationStake)
			}

			if accountData.LatestSupportedProtocolVersionAndHash != actualAccountData.LatestSupportedProtocolVersionAndHash {
				return ierrors.Errorf("AssertAccountData: %s: accountID %s expected latest supported protocol version and hash %d, got %d", node.Name, accountData.ID, accountData.LatestSupportedProtocolVersionAndHash, actualAccountData.LatestSupportedProtocolVersionAndHash)
			}
		}

		return nil
	})
}

func (t *TestSuite) AssertAccountDiff(accountID iotago.AccountID, index iotago.SlotIndex, accountDiff *model.AccountDiff, destroyed bool, nodes ...*mock.Node) {
	t.Eventually(func() error {
		for _, node := range nodes {

			accountsDiffStorage, err := node.Protocol.MainEngine.Get().Storage.AccountDiffs(index)
			if err != nil {
				return ierrors.Wrapf(err, "AssertAccountDiff: %s: failed to load accounts diff for slot %d", node.Name, index)
			}

			if has, err := accountsDiffStorage.Has(accountID); err != nil {
				return ierrors.Wrapf(err, "AssertAccountDiff: %s: failed to load accounts diff for slot %d", node.Name, index)
			} else if !has {
				return ierrors.Errorf("AssertAccountDiff: %s: accounts diff for slot %d does not contain account %s", node.Name, index, accountID)
			}

			actualAccountDiff, actualDestroyed, err := accountsDiffStorage.Load(accountID)
			if err != nil {
				return ierrors.Wrapf(err, "AssertAccountDiff: %s: failed to load account diff for account %s at slot %d", node.Name, accountID, index)
			}

			if destroyed != actualDestroyed {
				return ierrors.Errorf("AssertAccountDiff: %s: expected destroyed %t but actual %t for account %s at slot %d", node.Name, destroyed, actualDestroyed, accountID, index)
			}

			if accountDiff.BICChange != actualAccountDiff.BICChange {
				return ierrors.Errorf("AssertAccountDiff: %s: expected change %d but actual %d for account %s at slot %d", node.Name, accountDiff.BICChange, actualAccountDiff.BICChange, accountID, index)
			}

			if accountDiff.PreviousUpdatedTime != actualAccountDiff.PreviousUpdatedTime {
				return ierrors.Errorf("AssertAccountDiff: %s: expected previous updated time %d but actual %d for account %s at slot %d", node.Name, accountDiff.PreviousUpdatedTime, actualAccountDiff.PreviousUpdatedTime, accountID, index)
			}

			if accountDiff.NewExpirySlot != actualAccountDiff.NewExpirySlot {
				return ierrors.Errorf("AssertAccountDiff: %s: expected new expiry slot %d but actual %d for account %s at slot %d", node.Name, accountDiff.NewExpirySlot, actualAccountDiff.NewExpirySlot, accountID, index)
			}

			if accountDiff.PreviousExpirySlot != actualAccountDiff.PreviousExpirySlot {
				return ierrors.Errorf("AssertAccountDiff: %s: expected previous expiry slot %d but actual %d for account %s at slot %d", node.Name, accountDiff.PreviousExpirySlot, actualAccountDiff.PreviousExpirySlot, accountID, index)
			}

			if accountDiff.NewOutputID != actualAccountDiff.NewOutputID {
				return ierrors.Errorf("AssertAccountDiff: %s: expected new output ID %s but actual %s for account %s at slot %d", node.Name, accountDiff.NewOutputID, actualAccountDiff.NewOutputID, accountID, index)
			}

			if accountDiff.PreviousOutputID != actualAccountDiff.PreviousOutputID {
				return ierrors.Errorf("AssertAccountDiff: %s: expected previous output ID %s but actual %s for account %s at slot %d", node.Name, accountDiff.PreviousOutputID, actualAccountDiff.PreviousOutputID, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.BlockIssuerKeysAdded, actualAccountDiff.BlockIssuerKeysAdded) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected pub keys added %s but actual %s for account %s at slot %d", node.Name, accountDiff.BlockIssuerKeysAdded, actualAccountDiff.BlockIssuerKeysAdded, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.BlockIssuerKeysRemoved, actualAccountDiff.BlockIssuerKeysRemoved) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected pub keys removed %s but actual %s for account %s at slot %d", node.Name, accountDiff.BlockIssuerKeysRemoved, actualAccountDiff.BlockIssuerKeysRemoved, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.StakeEndEpochChange, actualAccountDiff.StakeEndEpochChange) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected new stake end epoch %d but actual %d for account %s at slot %d", node.Name, accountDiff.StakeEndEpochChange, actualAccountDiff.StakeEndEpochChange, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.FixedCostChange, actualAccountDiff.FixedCostChange) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected fixed cost change %d but actual %d for account %s at slot %d", node.Name, accountDiff.FixedCostChange, actualAccountDiff.FixedCostChange, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.ValidatorStakeChange, actualAccountDiff.ValidatorStakeChange) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected validator stake change epoch %d but actual %d for account %s at slot %d", node.Name, accountDiff.ValidatorStakeChange, actualAccountDiff.ValidatorStakeChange, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.DelegationStakeChange, actualAccountDiff.DelegationStakeChange) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected delegation stake change epoch %d but actual %d for account %s at slot %d", node.Name, accountDiff.DelegationStakeChange, actualAccountDiff.DelegationStakeChange, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.PrevLatestSupportedVersionAndHash, actualAccountDiff.PrevLatestSupportedVersionAndHash) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected previous latest supported protocol version change %d but actual %d for account %s at slot %d", node.Name, accountDiff.PreviousExpirySlot, actualAccountDiff.PrevLatestSupportedVersionAndHash, accountID, index)
			}

			if !assert.Equal(t.fakeTesting, accountDiff.NewLatestSupportedVersionAndHash, actualAccountDiff.NewLatestSupportedVersionAndHash) {
				return ierrors.Errorf("AssertAccountDiff: %s: expected new latest supported protocol version change %d but actual %d for account %s at slot %d", node.Name, accountDiff.NewLatestSupportedVersionAndHash, actualAccountDiff.NewLatestSupportedVersionAndHash, accountID, index)
			}
		}

		return nil
	})
}
