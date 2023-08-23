package network

import (
	"sync/atomic"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/crypto/identity"
)

const DefaultReconnectInterval = 5 * time.Second

// ConnectionDirection is an enum for the type of connection between local peer and the other peer in the gossip layer.
type ConnectionDirection string

const (
	// ConnDirectionOutbound means that the local peer dials for the connection in the gossip layer.
	ConnDirectionOutbound ConnectionDirection = "outbound"
	// ConnDirectionInbound means that the local peer accepts for the connection in the gossip layer.
	ConnDirectionInbound ConnectionDirection = "inbound"
)

// ConnectionStatus is an enum for the peer connection status in the gossip layer.
type ConnectionStatus string

const (
	// ConnStatusDisconnected means that there is no real connection established in the gossip layer for that peer.
	ConnStatusDisconnected ConnectionStatus = "disconnected"
	// ConnStatusConnected means that there is a real connection established in the gossip layer for that peer.
	ConnStatusConnected ConnectionStatus = "connected"
)

// PeerDescriptor defines a type that is used in .AddPeer() method.
type PeerDescriptor struct {
	PublicKey ed25519.PublicKey `json:"publicKey"`
	Addresses []ma.Multiaddr    `json:"addresses"`
}

// KnownPeer defines a peer record in the manual peering layer.
type KnownPeer struct {
	PublicKey     ed25519.PublicKey   `json:"publicKey"`
	Addresses     []ma.Multiaddr      `json:"addresses"`
	ConnDirection ConnectionDirection `json:"connectionDirection"`
	ConnStatus    ConnectionStatus    `json:"connectionStatus"`
}

type Peer struct {
	Identity      *identity.Identity
	PublicKey     ed25519.PublicKey
	PeerAddresses []ma.Multiaddr
	ConnDirection ConnectionDirection
	ConnStatus    *atomic.Value
	RemoveCh      chan struct{}
	DoneCh        chan struct{}
}

func NewPeer(p *PeerDescriptor, connDirection ConnectionDirection) (*Peer, error) {
	// TODO: remove hive.go Peer
	kp := &Peer{
		Identity:      identity.New(p.PublicKey),
		PeerAddresses: p.Addresses,
		ConnDirection: connDirection,
		ConnStatus:    &atomic.Value{},
		RemoveCh:      make(chan struct{}),
		DoneCh:        make(chan struct{}),
	}
	kp.SetConnStatus(ConnStatusDisconnected)

	return kp, nil
}

func (kp *Peer) GetConnStatus() ConnectionStatus {
	//nolint:forcetypeassert // we do not care
	return kp.ConnStatus.Load().(ConnectionStatus)
}

func (kp *Peer) SetConnStatus(cs ConnectionStatus) {
	kp.ConnStatus.Store(cs)
}
