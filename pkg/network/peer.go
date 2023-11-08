package network

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
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
	byteBuffer := stream.NewByteBuffer()

	if err := stream.WriteObjectWithSize(byteBuffer, p.ID, serializer.SeriLengthPrefixTypeAsUint16, func(id peer.ID) ([]byte, error) {
		return []byte(id), nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to write peer ID")
	}

	if err := stream.WriteCollection(byteBuffer, serializer.SeriLengthPrefixTypeAsByte, func() (elementsCount int, err error) {
		for _, addr := range p.PeerAddresses {
			if err = stream.WriteObjectWithSize(byteBuffer, addr, serializer.SeriLengthPrefixTypeAsUint16, func(m multiaddr.Multiaddr) ([]byte, error) {
				return m.Bytes(), nil
			}); err != nil {
				return 0, ierrors.Wrap(err, "failed to write peer address")
			}
		}

		return len(p.PeerAddresses), nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to write peer addresses")
	}

	return byteBuffer.Bytes()
}

func (p *Peer) String() string {
	return fmt.Sprintf("Peer{ID: %s, Addrs: %v, ConnStatus: %s}", p.ID, p.PeerAddresses, p.GetConnStatus())
}

// peerFromBytes parses a peer from a byte slice.
func peerFromBytes(bytes []byte) (*Peer, error) {
	p := &Peer{
		PeerAddresses: make([]multiaddr.Multiaddr, 0),
		ConnStatus:    &atomic.Value{},
		RemoveCh:      make(chan struct{}),
		DoneCh:        make(chan struct{}),
	}

	var err error
	byteReader := stream.NewByteReader(bytes)

	if p.ID, err = stream.ReadObjectWithSize(byteReader, serializer.SeriLengthPrefixTypeAsUint16, func(bytes []byte) (peer.ID, int, error) {
		id, err := peer.IDFromBytes(bytes)
		if err != nil {
			return "", 0, ierrors.Wrap(err, "failed to parse peerID")
		}

		return id, len(bytes), nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to read peer ID")
	}

	p.SetConnStatus(ConnStatusDisconnected)

	if err = stream.ReadCollection(byteReader, serializer.SeriLengthPrefixTypeAsByte, func(i int) error {
		addr, err := stream.ReadObjectWithSize(byteReader, serializer.SeriLengthPrefixTypeAsUint16, func(bytes []byte) (multiaddr.Multiaddr, int, error) {
			m, err := multiaddr.NewMultiaddrBytes(bytes)
			if err != nil {
				return nil, 0, ierrors.Wrap(err, "failed to parse peer address")
			}

			return m, len(bytes), nil
		})
		if err != nil {
			return ierrors.Wrap(err, "failed to read peer address")
		}

		p.PeerAddresses = append(p.PeerAddresses, addr)

		return nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to read peer addresses")
	}

	return p, nil
}
