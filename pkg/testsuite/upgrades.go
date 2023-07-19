package testsuite

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertEpochVersions(epochVersions map[iotago.Version]iotago.EpochIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {

			for version, expectedEpoch := range epochVersions {
				epochForVersion, exists := node.Protocol.MainEngineInstance().Storage.Settings().EpochForVersion(version)
				if !exists {
					return ierrors.Errorf("AssertEpochVersions: %s: version %d not found", node.Name, version)
				}

				if expectedEpoch != epochForVersion {
					return ierrors.Errorf("AssertEpochVersions: %s: for version %d epochs not equal. expected %d, got %d", node.Name, version, expectedEpoch, epochForVersion)
				}
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertVersionAndProtocolParameters(versionsAndProtocolParameters map[iotago.Version]iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {

			for version, expectedProtocolParameters := range versionsAndProtocolParameters {
				protocolParameters := node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(version)

				if expectedProtocolParameters == nil {
					if protocolParameters != nil {
						return ierrors.Errorf("AssertVersionAndProtocolParameters: %s: for version %d protocol parameters not equal. expected nil, got %s", node.Name, version, lo.PanicOnErr(protocolParameters.Hash()))
					}

					continue
				}

				if protocolParameters == nil {
					return ierrors.Errorf("AssertVersionAndProtocolParameters: %s: for version %d protocol parameters not equal. expected %s, got nil", node.Name, version, lo.PanicOnErr(expectedProtocolParameters.Hash()))
				}

				if lo.PanicOnErr(expectedProtocolParameters.Hash()) != lo.PanicOnErr(protocolParameters.Hash()) {
					return ierrors.Errorf("AssertVersionAndProtocolParameters: %s: for version %d protocol parameters not equal. expected %s, got %s", node.Name, version, lo.PanicOnErr(expectedProtocolParameters.Hash()), lo.PanicOnErr(protocolParameters.Hash()))
				}
			}

			return nil
		})
	}
}
