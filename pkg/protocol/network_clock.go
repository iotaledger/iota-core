package protocol

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
)

type NetworkClock struct {
	protocol *Protocol

	reactive.Variable[time.Time]
}

func NewNetworkClock(protocol *Protocol) *NetworkClock {
	n := &NetworkClock{
		protocol: protocol,
		Variable: reactive.NewVariable[time.Time](func(currentValue time.Time, newValue time.Time) time.Time {
			if newValue.Before(currentValue) || newValue.After(time.Now()) {
				return currentValue
			} else {
				return newValue
			}
		}),
	}

	protocol.Constructed.OnTrigger(func() {
		unsubscribe := lo.Batch(
			protocol.Network.OnBlockReceived(func(block *model.Block, src peer.ID) {
				n.Set(block.ProtocolBlock().IssuingTime)
			}),

			protocol.OnChainCreated(func(chain *Chain) {
				unsubscribe := chain.LatestCommitment.OnUpdate(func(_, latestCommitment *Commitment) {
					if engineInstance := chain.Engine.Get(); engineInstance != nil {
						n.Set(engineInstance.LatestAPI().TimeProvider().SlotEndTime(latestCommitment.Slot()))
					}
				})

				chain.IsEvicted.OnTrigger(unsubscribe)
			}),
		)

		protocol.Shutdown.OnTrigger(unsubscribe)
	})

	return n
}
