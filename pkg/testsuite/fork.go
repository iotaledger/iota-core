package testsuite

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertForkDetectedCount(expectedCount int, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			actualCount := node.ForkDetectedCount()
			if expectedCount != actualCount {
				return ierrors.Errorf("AssertForkDetectedCount: %s: expected %v, got %v", node.Name, expectedCount, actualCount)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertCandidateEngineActivatedCount(expectedCount int, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			actualCount := node.CandidateEngineActivatedCount()
			if expectedCount != actualCount {
				return ierrors.Errorf("AssertCandidateEngineActivatedCount: %s: expected %v, got %v", node.Name, expectedCount, actualCount)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertMainEngineSwitchedCount(expectedCount int, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			actualCount := node.MainEngineSwitchedCount()
			if expectedCount != actualCount {
				return ierrors.Errorf("AssertMainEngineSwitchedCount: %s: expected %v, got %v", node.Name, expectedCount, actualCount)
			}

			return nil
		})
	}
}
