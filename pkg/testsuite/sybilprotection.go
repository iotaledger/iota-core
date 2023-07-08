package testsuite

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSybilProtectionCommittee(slotIndex iotago.SlotIndex, expectedAccounts []iotago.AccountID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			accounts := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(slotIndex).Accounts().IDs()
			if !assert.ElementsMatch(t.fakeTesting, expectedAccounts, accounts) {
				return errors.Errorf("AssertSybilProtectionCommittee: %s: expected %s, got %s", node.Name, expectedAccounts, accounts)
			}

			if len(expectedAccounts) != len(accounts) {
				return errors.Errorf("AssertSybilProtectionCommittee: %s: expected %v, got %v", node.Name, len(expectedAccounts), len(accounts))
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
				return errors.Errorf("AssertSybilProtectionOnlineCommittee: %s: expected %v, got %v", node.Name, expectedSeats, seats)
			}

			return nil
		})
	}
}
