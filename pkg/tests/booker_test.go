package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func TestBooker(t *testing.T) {
	ts := testsuite.NewTestSuite(t, testsuite.WithSnapshotOptions(snapshotcreator.WithLedgerProvider(mock.NewMockedLedgerProvider())))
	defer ts.Shutdown()

	ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithLedgerProvider(mock.NewMockedLedgerProvider()),
		},
	})

	//ts.Node("node1").IssueBlock(mempooltests.NewTransaction(2, inputs...	iotago.IndexedUTXOReferencer))
}
