package mock

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/network"
)

// region Network ////////////////////////////////////////////////////////////////////////////////////////////////

const NetworkMainPartition = "main"

type Network struct {
	dispatchersByPartition map[string]map[network.PeerID]*Endpoint
	dispatchersMutex       sync.RWMutex
}

func NewNetwork() *Network {
	return &Network{
		dispatchersByPartition: map[string]map[network.PeerID]*Endpoint{
			NetworkMainPartition: make(map[network.PeerID]*Endpoint),
		},
	}
}

func (n *Network) Join(endpointID network.PeerID, partition string) *Endpoint {
	n.dispatchersMutex.Lock()
	defer n.dispatchersMutex.Unlock()

	endpoint := newMockedEndpoint(endpointID, n, partition)

	dispatchers, exists := n.dispatchersByPartition[partition]
	if !exists {
		dispatchers = make(map[network.PeerID]*Endpoint)
		n.dispatchersByPartition[partition] = dispatchers
	}
	dispatchers[endpointID] = endpoint

	return endpoint
}

func (n *Network) MergePartitionsToMain(partitions ...string) {
	n.dispatchersMutex.Lock()
	defer n.dispatchersMutex.Unlock()

	switch {
	case len(partitions) == 0:
		// Merge all partitions to main
		for partitionID := range n.dispatchersByPartition {
			if partitionID != NetworkMainPartition {
				n.mergePartition(partitionID)
			}
		}
	default:
		for _, partitionID := range partitions {
			n.mergePartition(partitionID)
		}
	}
}

func (n *Network) mergePartition(partitionID string) {
	for _, endpoint := range n.dispatchersByPartition[partitionID] {
		endpoint.partition = NetworkMainPartition
		n.dispatchersByPartition[NetworkMainPartition][endpoint.id] = endpoint
	}
	delete(n.dispatchersByPartition, partitionID)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Endpoint ///////////////////////////////////////////////////////////////////////////////////////////////

type Endpoint struct {
	id            network.PeerID
	network       *Network
	partition     string
	handlers      map[string]func(network.PeerID, proto.Message) error
	handlersMutex sync.RWMutex
}

func newMockedEndpoint(id network.PeerID, n *Network, partition string) *Endpoint {
	return &Endpoint{
		id:        id,
		network:   n,
		partition: partition,
		handlers:  make(map[string]func(network.PeerID, proto.Message) error),
	}
}

func (e *Endpoint) RegisterProtocol(protocolID string, _ func() proto.Message, handler func(network.PeerID, proto.Message) error) {
	e.handlersMutex.Lock()
	defer e.handlersMutex.Unlock()

	e.handlers[protocolID] = handler
}

func (e *Endpoint) UnregisterProtocol(protocolID string) {
	e.handlersMutex.Lock()
	defer e.handlersMutex.Unlock()

	delete(e.handlers, protocolID)

	e.network.dispatchersMutex.Lock()
	defer e.network.dispatchersMutex.Unlock()

	delete(e.network.dispatchersByPartition[e.partition], e.id)
}

func (e *Endpoint) Send(packet proto.Message, protocolID string, to ...network.PeerID) {
	e.network.dispatchersMutex.RLock()
	defer e.network.dispatchersMutex.RUnlock()

	if len(to) == 0 {
		to = lo.Keys(e.network.dispatchersByPartition[e.partition])
	}

	for _, id := range to {
		if id == e.id {
			continue
		}

		dispatcher, exists := e.network.dispatchersByPartition[e.partition][id]
		if !exists {
			fmt.Println(e.id, "ERROR: no dispatcher for ", id)
			continue
		}

		protocolHandler, exists := dispatcher.handler(protocolID)
		if !exists {
			fmt.Println(e.id, "ERROR: no protocol handler for", protocolID, id)
			continue
		}

		if err := protocolHandler(e.id, packet); err != nil {
			fmt.Println(e.id, "ERROR: ", err)
		}
	}

}

func (e *Endpoint) handler(protocolID string) (handler func(network.PeerID, proto.Message) error, exists bool) {
	e.handlersMutex.RLock()
	defer e.handlersMutex.RUnlock()

	handler, exists = e.handlers[protocolID]
	return
}

var _ network.Endpoint = &Endpoint{}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
