package tests

import (
	"fmt"
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TopStakersRotation(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithSnapshotOptions(
			snapshotcreator.WithSeatManagerProvider(
				topstakers.NewProvider(
					topstakers.WithSeatCount(3),
				),
			),
		),
		testsuite.WithGenesisTimestampOffset(100*10),
	)
	defer ts.Shutdown()

	_ = ts.AddValidatorNode("node1", 1_000_002)
	_ = ts.AddValidatorNode("node2", 1_000_001)
	node3 := ts.AddValidatorNode("node3", 1_000_000)
	node4 := ts.AddValidatorNode("node4")
	node5 := ts.AddValidatorNode("node5")
	node6 := ts.AddValidatorNode("node6")

	blockIssuer := ts.AddBasicBlockIssuer("default")
	nodeOptions := make(map[string][]options.Option[protocol.Protocol])

	for _, node := range ts.Nodes() {
		nodeOptions[node.Name] = []options.Option[protocol.Protocol]{protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(
						topstakers.WithSeatCount(3),
					),
				),
			),
		)}
	}
	ts.Run(true, nodeOptions)

	for _, node := range ts.Nodes() {
		nodeOptions[node.Name] = []options.Option[protocol.Protocol]{protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(topstakers.WithSeatCount(3)),
				),
			),
		)}
	}

	// TODO: replace with CandidacyPayload
	ts.IssuePayloadWithOptions("block1", blockIssuer, node6, &iotago.TaggedData{
		Tag:  nil,
		Data: nil,
	}, mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(1)))

	ts.IssuePayloadWithOptions("block2", blockIssuer, node5, &iotago.TaggedData{
		Tag:  nil,
		Data: nil,
	}, mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(1)))

	ts.IssuePayloadWithOptions("block3", blockIssuer, node4, &iotago.TaggedData{
		Tag:  nil,
		Data: nil,
	}, mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(1)))

	ts.IssuePayloadWithOptions("block4", blockIssuer, node3, &iotago.TaggedData{
		Tag:  nil,
		Data: nil,
	}, mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(1)))
	fmt.Println("commit until", ts.API.TimeProvider().EpochEnd(0)-ts.API.ProtocolParameters().EpochNearingThreshold())
	ts.CommitUntilSlot(ts.API.TimeProvider().EpochEnd(0)-ts.API.ProtocolParameters().EpochNearingThreshold(), ts.Block("block4"))
}
