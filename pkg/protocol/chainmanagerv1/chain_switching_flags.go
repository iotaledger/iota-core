package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type chainSwitchingFlags struct {
	requestAttestations reactive.Variable[bool]
}

func newChainSwitchingFlags(chainManager *Chain) *chainSwitchingFlags {
	return &chainSwitchingFlags{
		requestAttestations: reactive.NewVariable[bool](),
	}
}

func (c *chainSwitchingFlags) RequestAttestations() reactive.Variable[bool] {
	return c.requestAttestations
}
