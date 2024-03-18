package protocol

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
)

var ErrBlockTimeTooFarAheadInFuture = ierrors.New("a block cannot be too far ahead in the future")

// Network is a subcomponent of the protocol that is responsible for handling the network communication.
type Network struct {
	// Protocol contains the network endpoint of the protocol.
	*core.Protocol

	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// Logger contains a reference to the logger that is used by this component.
	log.Logger
}

// newNetwork creates a new network protocol instance for the given protocol and network endpoint.
func newNetwork(protocol *Protocol, networkEndpoint network.Endpoint) *Network {
	n := &Network{
		Protocol: core.NewProtocol(networkEndpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),
		Logger:   protocol.NewChildLogger("Network"),
		protocol: protocol,
	}

	protocol.ShutdownEvent().OnTrigger(n.Logger.UnsubscribeFromParentLogger)

	return n
}

// OnBlockReceived overwrites the OnBlockReceived method of the core protocol to filter out invalid blocks.
func (n *Network) OnBlockReceived(callback func(block *model.Block, src peer.ID)) (unsubscribe func()) {
	return n.Protocol.OnBlockReceived(func(block *model.Block, src peer.ID) {
		// filter blocks from the future
		if timeDelta := time.Since(block.ProtocolBlock().Header.IssuingTime); timeDelta < -n.protocol.Options.MaxAllowedWallClockDrift {
			n.LogWarn("filtered block, issuing time ahead", "block", block.ID(), "issuingTime", block.ProtocolBlock().Header.IssuingTime, "timeDelta", timeDelta, "deltaAllowed", n.protocol.Options.MaxAllowedWallClockDrift, "from", src, "err", ErrBlockTimeTooFarAheadInFuture)

			return
		}

		callback(block, src)
	})
}
