package testsuite

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSybilProtectionCommittee(epoch iotago.EpochIndex, expectedAccounts []iotago.AccountID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			committeeInEpoch, exists := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInEpoch(epoch)
			if !exists {
				return ierrors.Errorf("AssertSybilProtectionCommittee: %s: failed to get committee in epoch %d", node.Name, epoch)
			}

			committeeInEpochAccounts, err := committeeInEpoch.Accounts()
			if err != nil {
				return ierrors.Errorf("AssertSybilProtectionCommittee: %s: failed to get accounts in committee in epoch %d: %w", node.Name, epoch, err)
			}

			accountIDs := committeeInEpochAccounts.IDs()
			if !assert.ElementsMatch(t.fakeTesting, expectedAccounts, accountIDs) {
				return ierrors.Errorf("AssertSybilProtectionCommittee: %s: expected %s, got %s", node.Name, expectedAccounts, accountIDs)
			}

			if len(expectedAccounts) != len(accountIDs) {
				return ierrors.Errorf("AssertSybilProtectionCommittee: %s: expected %v, got %v", node.Name, len(expectedAccounts), len(accountIDs))
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertSybilProtectionCandidates(epoch iotago.EpochIndex, expectedAccounts []iotago.AccountID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			candidates, err := node.Protocol.MainEngineInstance().SybilProtection.EligibleValidators(epoch)
			candidateIDs := lo.Map(candidates, func(candidate *accounts.AccountData) iotago.AccountID {
				return candidate.ID
			})
			require.NoError(t.Testing, err)

			if !assert.ElementsMatch(t.fakeTesting, expectedAccounts, candidateIDs) {
				return ierrors.Errorf("AssertSybilProtectionCandidates: %s: expected %s, got %s", node.Name, expectedAccounts, candidateIDs)
			}

			if len(expectedAccounts) != len(candidates) {
				return ierrors.Errorf("AssertSybilProtectionCandidates: %s: expected %v, got %v", node.Name, len(expectedAccounts), len(candidateIDs))
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertSybilProtectionOnlineCommittee(expectedSeats []account.SeatIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			seats := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().OnlineCommittee().ToSlice()
			if !assert.ElementsMatch(t.fakeTesting, expectedSeats, seats) {
				return ierrors.Errorf("AssertSybilProtectionOnlineCommittee: %s: expected %v, got %v", node.Name, expectedSeats, seats)
			}

			return nil
		})
	}
}
