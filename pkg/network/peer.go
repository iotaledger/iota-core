package network

import (
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	p2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/crypto/identity"
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
	PublicKey  ed25519.PublicKey `json:"publicKey"`
	Addresses  []ma.Multiaddr    `json:"addresses"`
	ConnStatus ConnectionStatus  `json:"connectionStatus"`
}

type Peer struct {
	Identity      *identity.Identity
	PublicKey     ed25519.PublicKey
	PeerAddresses []ma.Multiaddr
	ConnStatus    *atomic.Value
	RemoveCh      chan struct{}
	DoneCh        chan struct{}
}

func NewPeer(peerAddr ma.Multiaddr) (*Peer, error) {
	// TODO: remove hive.go Peer
	addrInfo, err := p2ppeer.AddrInfoFromP2pAddr(peerAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse p2p multiaddress")
	}

	icPubKey, err := addrInfo.ID.ExtractPublicKey()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to extract public key from peer ID")
	}

	ed25519PubKey, ok := icPubKey.(*crypto.Ed25519PublicKey)
	if !ok {
		return nil, ierrors.New("failed to cast public key to Ed25519")
	}

	ed25519PubKeyBytes, err := ed25519PubKey.Raw()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get raw public key bytes")
	}

	ed25519PubKeyNative, _, err := ed25519.PublicKeyFromBytes(ed25519PubKeyBytes)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get native public key")
	}

	kp := &Peer{
		Identity:      identity.New(ed25519PubKeyNative),
		PeerAddresses: []ma.Multiaddr{peerAddr},
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
