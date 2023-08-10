package mock

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/network"
)

// region network ////////////////////////////////////////////////////////////////////////////////////////////////

const NetworkMainPartition = "main"

type Network struct {
	dispatchersByPartition map[string]map[network.PeerID]*Endpoint
	dispatchersMutex       syncutils.RWMutex
}

func NewNetwork() *Network {
	return &Network{
		dispatchersByPartition: map[string]map[network.PeerID]*Endpoint{
			NetworkMainPartition: make(map[network.PeerID]*Endpoint),
		},
	}
}

func (n *Network) JoinWithEndpointID(endpointID network.PeerID, partition string) *Endpoint {
	return n.JoinWithEndpoint(newMockedEndpoint(endpointID, n, partition), partition)
}

func (n *Network) JoinWithEndpoint(endpoint *Endpoint, newPartition string) *Endpoint {
	n.dispatchersMutex.Lock()
	defer n.dispatchersMutex.Unlock()

	if endpoint.partition != newPartition {
		n.deleteEndpointFromPartition(endpoint, endpoint.partition)
	}

	n.addEndpointToPartition(endpoint, newPartition)

	return endpoint
}

func (n *Network) addEndpointToPartition(endpoint *Endpoint, newPartition string) {
	endpoint.partition = newPartition
	dispatchers, exists := n.dispatchersByPartition[newPartition]
	if !exists {
		dispatchers = make(map[network.PeerID]*Endpoint)
		n.dispatchersByPartition[newPartition] = dispatchers
	}
	dispatchers[endpoint.id] = endpoint
}

func (n *Network) deleteEndpointFromPartition(endpoint *Endpoint, partition string) {
	endpoint.partition = ""
	delete(n.dispatchersByPartition[partition], endpoint.id)

	if len(n.dispatchersByPartition[partition]) == 0 {
		delete(n.dispatchersByPartition, partition)
	}
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
		for _, partition := range partitions {
			n.mergePartition(partition)
		}
	}
}

func (n *Network) mergePartition(partition string) {
	for _, endpoint := range n.dispatchersByPartition[partition] {
		n.addEndpointToPartition(endpoint, NetworkMainPartition)
	}
	delete(n.dispatchersByPartition, partition)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Endpoint ///////////////////////////////////////////////////////////////////////////////////////////////

type Endpoint struct {
	id            network.PeerID
	network       *Network
	partition     string
	handlers      map[string]func(network.PeerID, proto.Message) error
	handlersMutex syncutils.RWMutex
}

func newMockedEndpoint(id network.PeerID, n *Network, partition string) *Endpoint {
	return &Endpoint{
		id:        id,
		network:   n,
		partition: partition,
		handlers:  make(map[string]func(network.PeerID, proto.Message) error),
	}
}

func (e *Endpoint) LocalPeerID() network.PeerID {
	return e.id
}

func (e *Endpoint) RegisterProtocol(protocolID string, _ func() proto.Message, handler func(network.PeerID, proto.Message) error) {
	e.handlersMutex.Lock()
	defer e.handlersMutex.Unlock()

	e.handlers[protocolID] = handler
}

func (e *Endpoint) UnregisterProtocol(protocolID string) {
	e.network.dispatchersMutex.Lock()
	defer e.network.dispatchersMutex.Unlock()

	e.handlersMutex.Lock()
	defer e.handlersMutex.Unlock()

	delete(e.handlers, protocolID)
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

		go func() {
			if err := protocolHandler(e.id, packet); err != nil {
				fmt.Println(e.id, "ERROR: ", err)
			}
		}()

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
