package testsuite

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSybilProtectionCommittee(weightVector map[iotago.AccountID]int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.assertSybilProtectionCustomCommittee("Committee", weightVector, node.Protocol.MainEngineInstance().SybilProtection.Committee, node)
	}
}

func (t *TestSuite) AssertSybilProtectionOnlineCommittee(weightVector map[iotago.AccountID]int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.assertSybilProtectionCustomCommittee("Online", weightVector, node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee, node)
	}
}

func (t *TestSuite) assertSybilProtectionCustomCommittee(customCommitteeType string, weightVector map[iotago.AccountID]int64, committeeFunc func() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID], node *mock.Node) {
	t.Eventually(func() error {
		if !assert.ElementsMatch(t.fakeTesting, lo.Keys(weightVector), committeeFunc().Members().Slice()) {
			return errors.Errorf("AssertSybilProtectionCommittee(%s): %s: expected %s, got %s", customCommitteeType, node.Name, lo.Keys(weightVector), committeeFunc().Members().Slice())
		}
		committee := committeeFunc()
		fmt.Println("committee", committee.Members().Slice(), committee.TotalWeight())

		if lo.Sum(lo.Values(weightVector)...) != committeeFunc().TotalWeight() {
			return errors.Errorf("AssertSybilProtectionCommittee(%s): %s: expected %v, got %v", customCommitteeType, node.Name, lo.Sum(lo.Values(weightVector)...), committeeFunc().TotalWeight())
		}

		err := committeeFunc().ForEach(func(accountID iotago.AccountID, weight int64) error {
			if weightVector[accountID] != weight {
				return errors.Errorf("AssertSybilProtectionCommittee(%s): %s: expected %d, got %d for %s", customCommitteeType, node.Name, weightVector[accountID], weight, accountID)
			}

			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})
}
