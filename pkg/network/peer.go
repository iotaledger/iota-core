package network

import (
	"sync/atomic"
	"time"

	p2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
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

// KnownPeer defines a peer record in the manual peering layer.
type KnownPeer struct {
	Addresses  []ma.Multiaddr   `json:"addresses"`
	ConnStatus ConnectionStatus `json:"connectionStatus"`
}

type Peer struct {
	ID            p2ppeer.ID
	PublicKey     ed25519.PublicKey
	PeerAddresses []ma.Multiaddr
	ConnStatus    *atomic.Value
	RemoveCh      chan struct{}
	DoneCh        chan struct{}
}

func NewPeerFromAddrInfo(addrInfo *p2ppeer.AddrInfo) (*Peer, error) {
	kp := &Peer{
		ID:            addrInfo.ID,
		PeerAddresses: addrInfo.Addrs,
		ConnStatus:    &atomic.Value{},
		RemoveCh:      make(chan struct{}),
		DoneCh:        make(chan struct{}),
	}
	kp.SetConnStatus(ConnStatusDisconnected)

	return kp, nil
}

func NewPeerFromMultiAddr(peerAddrs ma.Multiaddr) (*Peer, error) {
	addrInfo, err := p2ppeer.AddrInfoFromP2pAddr(peerAddrs)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse p2p multiaddress")
	}

	return NewPeerFromAddrInfo(addrInfo)
}

func (kp *Peer) GetConnStatus() ConnectionStatus {
	//nolint:forcetypeassert // we do not care
	return kp.ConnStatus.Load().(ConnectionStatus)
}

func (kp *Peer) SetConnStatus(cs ConnectionStatus) {
	kp.ConnStatus.Store(cs)
}
