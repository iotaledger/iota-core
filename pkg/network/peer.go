package network

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
)

const DefaultReconnectInterval = 5 * time.Second

// ConnectionStatus is an enum for the peer connection status in the gossip layer.
type ConnectionStatus string

const (
	// ConnStatusDisconnected means that there is no real connection established in the gossip layer for that peer.
	ConnStatusDisconnected ConnectionStatus = "disconnected"
	// ConnStatusConnected means that there is a real connection established in the gossip layer for that peer.
	ConnStatusConnected ConnectionStatus = "connected"
)

// PeerDescriptor defines a peer record in the manual peering layer.
type PeerDescriptor struct {
	Addresses []multiaddr.Multiaddr `json:"addresses"`
}

type Peer struct {
	ID            peer.ID
	PublicKey     ed25519.PublicKey
	PeerAddresses []multiaddr.Multiaddr
	ConnStatus    *atomic.Value
	RemoveCh      chan struct{}
	DoneCh        chan struct{}
}

func NewPeerFromAddrInfo(addrInfo *peer.AddrInfo) *Peer {
	p := &Peer{
		ID:            addrInfo.ID,
		PeerAddresses: addrInfo.Addrs,
		ConnStatus:    &atomic.Value{},
		RemoveCh:      make(chan struct{}),
		DoneCh:        make(chan struct{}),
	}
	p.SetConnStatus(ConnStatusDisconnected)

	return p
}

func NewPeerFromMultiAddr(peerAddrs multiaddr.Multiaddr) (*Peer, error) {
	addrInfo, err := peer.AddrInfoFromP2pAddr(peerAddrs)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse p2p multiaddress")
	}

	return NewPeerFromAddrInfo(addrInfo), nil
}

func (p *Peer) ToAddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    p.ID,
		Addrs: p.PeerAddresses,
	}
}

func (p *Peer) GetConnStatus() ConnectionStatus {
	//nolint:forcetypeassert // we do not care
	return p.ConnStatus.Load().(ConnectionStatus)
}

func (p *Peer) SetConnStatus(cs ConnectionStatus) {
	p.ConnStatus.Store(cs)
}

func (p *Peer) Bytes() ([]byte, error) {
	m := marshalutil.New()
	m.WriteUint64(uint64(len(p.ID)))
	m.WriteBytes([]byte(p.ID))
	m.WriteUint8(uint8(len(p.PeerAddresses)))
	for _, addr := range p.PeerAddresses {
		addrBytes := addr.Bytes()
		m.WriteUint64(uint64(len(addrBytes)))
		m.WriteBytes(addrBytes)
	}

	return m.Bytes(), nil
}

func (p *Peer) String() string {
	return fmt.Sprintf("Peer{ID: %s, Addrs: %v, ConnStatus: %s}", p.ID, p.PeerAddresses, p.GetConnStatus())
}

// peerFromBytes parses a peer from a byte slice.
func peerFromBytes(bytes []byte) (*Peer, error) {
	m := marshalutil.New(bytes)
	idLen, err := m.ReadUint64()
	if err != nil {
		return nil, err
	}
	idBytes, err := m.ReadBytes(int(idLen))
	if err != nil {
		return nil, err
	}
	id := peer.ID(idBytes)

	peer := &Peer{
		ID:            id,
		PeerAddresses: make([]multiaddr.Multiaddr, 0),
		ConnStatus:    &atomic.Value{},
		RemoveCh:      make(chan struct{}),
		DoneCh:        make(chan struct{}),
	}

	peer.SetConnStatus(ConnStatusDisconnected)

	peerAddrLen, err := m.ReadUint8()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(peerAddrLen); i++ {
		addrLen, err := m.ReadUint64()
		if err != nil {
			return nil, err
		}
		addrBytes, err := m.ReadBytes(int(addrLen))
		if err != nil {
			return nil, err
		}
		addr, err := multiaddr.NewMultiaddrBytes(addrBytes)
		if err != nil {
			return nil, err
		}
		peer.PeerAddresses = append(peer.PeerAddresses, addr)
	}

	return peer, nil
}
