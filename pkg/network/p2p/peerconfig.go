package p2p

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/ds/onchangemap"
	"github.com/iotaledger/hive.go/lo"
)

// ComparablePeerID implements the constraints.ComparableStringer interface for the onChangeMap.
type ComparablePeerID struct {
	peerIDBase58 string
}

func NewComparablePeerID(peerID peer.ID) *ComparablePeerID {
	return &ComparablePeerID{
		peerIDBase58: peerID.String(),
	}
}

func (c *ComparablePeerID) Key() string {
	return c.peerIDBase58
}

func (c *ComparablePeerID) String() string {
	return c.peerIDBase58
}

// PeerConfig holds the initial information about peers.
type PeerConfig struct {
	MultiAddress string `json:"multiAddress" koanf:"multiAddress"`
	Alias        string `json:"alias" koanf:"alias"`
}

// PeerConfigItem implements the Item interface for the onChangeMap.
type PeerConfigItem struct {
	*PeerConfig
	comparablePeerID *ComparablePeerID
}

func NewPeerConfigItem(peerConfig *PeerConfig) (*PeerConfigItem, error) {
	multiAddress, err := multiaddr.NewMultiaddr(peerConfig.MultiAddress)
	if err != nil {
		return nil, err
	}

	newPeerAddrInfo, err := peer.AddrInfoFromP2pAddr(multiAddress)
	if err != nil {
		return nil, err
	}

	return &PeerConfigItem{
		PeerConfig: &PeerConfig{
			MultiAddress: peerConfig.MultiAddress,
			Alias:        peerConfig.Alias,
		},
		comparablePeerID: NewComparablePeerID(newPeerAddrInfo.ID),
	}, nil
}

func (p *PeerConfigItem) ID() *ComparablePeerID {
	return p.comparablePeerID
}

func (p *PeerConfigItem) Clone() onchangemap.Item[string, *ComparablePeerID] {
	return lo.PanicOnErr(NewPeerConfigItem(p.PeerConfig))
}
