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

func (m *Network) Join(endpointID network.PeerID, partition string) *Endpoint {
	m.dispatchersMutex.Lock()
	defer m.dispatchersMutex.Unlock()

	endpoint := newMockedEndpoint(endpointID, m, partition)

	dispatchers, exists := m.dispatchersByPartition[partition]
	if !exists {
		dispatchers = make(map[network.PeerID]*Endpoint)
		m.dispatchersByPartition[partition] = dispatchers
	}
	dispatchers[endpointID] = endpoint

	return endpoint
}

func (m *Network) MergePartitionsToMain(partitions ...string) {
	m.dispatchersMutex.Lock()
	defer m.dispatchersMutex.Unlock()

	switch {
	case len(partitions) == 0:
		// Merge all partitions to main
		for partitionID := range m.dispatchersByPartition {
			if partitionID != NetworkMainPartition {
				m.mergePartition(partitionID)
			}
		}
	default:
		for _, partitionID := range partitions {
			m.mergePartition(partitionID)
		}
	}
}

func (m *Network) mergePartition(partitionID string) {
	for _, endpoint := range m.dispatchersByPartition[partitionID] {
		endpoint.partition = NetworkMainPartition
		m.dispatchersByPartition[NetworkMainPartition][endpoint.id] = endpoint
	}
	delete(m.dispatchersByPartition, partitionID)
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

func newMockedEndpoint(id network.PeerID, n *Network, partition string) (newMockedNetwork *Endpoint) {
	return &Endpoint{
		id:        id,
		network:   n,
		partition: partition,
		handlers:  make(map[string]func(network.PeerID, proto.Message) error),
	}
}

func (m *Endpoint) RegisterProtocol(protocolID string, _ func() proto.Message, handler func(network.PeerID, proto.Message) error) {
	m.handlersMutex.Lock()
	defer m.handlersMutex.Unlock()

	m.handlers[protocolID] = handler
}

func (m *Endpoint) UnregisterProtocol(protocolID string) {
	m.handlersMutex.Lock()
	defer m.handlersMutex.Unlock()

	delete(m.handlers, protocolID)
}

func (m *Endpoint) Send(packet proto.Message, protocolID string, to ...network.PeerID) {
	m.network.dispatchersMutex.RLock()
	defer m.network.dispatchersMutex.RUnlock()

	if len(to) == 0 {
		to = lo.Keys(m.network.dispatchersByPartition[m.partition])
	}

	for _, id := range to {
		if id == m.id {
			continue
		}

		dispatcher, exists := m.network.dispatchersByPartition[m.partition][id]
		if !exists {
			fmt.Println("ERROR: no dispatcher for ", id)
		}

		protocolHandler, exists := dispatcher.handler(protocolID)
		if !exists {
			fmt.Println("ERROR: no protocol handler for ", protocolID)
		}

		if err := protocolHandler(m.id, packet); err != nil {
			fmt.Println("ERROR: ", err)
		}
	}

}

func (m *Endpoint) handler(protocolID string) (handler func(network.PeerID, proto.Message) error, exists bool) {
	m.handlersMutex.RLock()
	defer m.handlersMutex.RUnlock()

	handler, exists = m.handlers[protocolID]
	return
}

var _ network.Endpoint = &Endpoint{}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
