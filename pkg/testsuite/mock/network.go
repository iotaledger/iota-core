package mock

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/network"
)

// region network ////////////////////////////////////////////////////////////////////////////////////////////////

const NetworkMainPartition = "main"

type Network struct {
	dispatchersByPartition map[string]map[peer.ID]*Endpoint
	dispatchersMutex       syncutils.RWMutex
}

func NewNetwork() *Network {
	return &Network{
		dispatchersByPartition: map[string]map[peer.ID]*Endpoint{
			NetworkMainPartition: make(map[peer.ID]*Endpoint),
		},
	}
}

func (n *Network) JoinWithEndpointID(endpointID peer.ID, partition string) *Endpoint {
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
		dispatchers = make(map[peer.ID]*Endpoint)
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
	id        peer.ID
	network   *Network
	partition string
	handler   func(peer.ID, proto.Message) error
}

func newMockedEndpoint(id peer.ID, n *Network, partition string) *Endpoint {
	return &Endpoint{
		id:        id,
		network:   n,
		partition: partition,
	}
}

func (e *Endpoint) LocalPeerID() peer.ID {
	return e.id
}

func (e *Endpoint) RegisterProtocol(_ func() proto.Message, handler func(peer.ID, proto.Message) error) {
	e.handler = handler
}

func (e *Endpoint) UnregisterProtocol() {
	e.network.dispatchersMutex.Lock()
	defer e.network.dispatchersMutex.Unlock()

	e.handler = nil
	delete(e.network.dispatchersByPartition[e.partition], e.id)
}

func (e *Endpoint) Shutdown() {
	e.UnregisterProtocol()
}

func (e *Endpoint) Send(packet proto.Message, to ...peer.ID) {
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

		go func() {
			e.network.dispatchersMutex.RLock()
			defer e.network.dispatchersMutex.RUnlock()

			if err := dispatcher.handler(e.id, packet); err != nil {
				fmt.Println(e.id, "ERROR: ", err)
			}
		}()
	}
}

var _ network.Endpoint = &Endpoint{}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
