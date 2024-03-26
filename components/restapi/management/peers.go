package management

import (
	"sort"

	"github.com/labstack/echo/v4"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	// PeerRelationManual is the relation of a manually peered peer.
	PeerRelationManual string = "manual"
	// PeerRelationAutopeered is the relation of an autopeered peer.
	PeerRelationAutopeered string = "autopeered"
)

// parsePeerIDParam parses the peerID parameter from the request.
func parsePeerIDParam(c echo.Context) (peer.ID, error) {
	peerID, err := peer.Decode(c.Param(api.ParameterPeerID))
	if err != nil {
		return "", ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid peerID: %w", err)
	}

	return peerID, nil
}

// getPeerInfoFromNeighbor returns the peer info for the given peer.
func getPeerInfoFromPeer(peer *network.Peer) *api.PeerInfo {
	multiAddresses := make([]iotago.PrefixedStringUint8, len(peer.PeerAddresses))
	for i, multiAddress := range peer.PeerAddresses {
		multiAddresses[i] = iotago.PrefixedStringUint8(multiAddress.String())
	}

	var alias string
	relation := PeerRelationAutopeered

	if peerConfigItem := deps.PeeringConfigManager.Peer(peer.ID); peerConfigItem != nil {
		alias = peerConfigItem.Alias

		// if the peer exists in the config, it is a manual peered peer
		relation = PeerRelationManual
	}

	packetsReceived := uint32(0)
	packetsSent := uint32(0)

	if neighbor, err := deps.NetworkManager.Neighbor(peer.ID); err == nil {
		packetsReceived = uint32(neighbor.PacketsRead())
		packetsSent = uint32(neighbor.PacketsWritten())
	}

	return &api.PeerInfo{
		ID:             peer.ID.String(),
		MultiAddresses: multiAddresses,
		Alias:          alias,
		Relation:       relation,
		Connected:      peer.ConnStatus.Load() == network.ConnStatusConnected,
		GossipMetrics: &api.PeerGossipMetrics{
			PacketsReceived: packetsReceived,
			PacketsSent:     packetsSent,
		},
	}
}

// getPeer returns the peer info for the given peerID in the request.
func getPeer(c echo.Context) (*api.PeerInfo, error) {
	peerID, err := parsePeerIDParam(c)
	if err != nil {
		return nil, err
	}

	// check connected neighbors first
	if neighbor, err := deps.NetworkManager.Neighbor(peerID); err == nil {
		return getPeerInfoFromPeer(neighbor.Peer()), nil
	}

	// if the peer is not connected, check the manual peers
	peer, err := deps.NetworkManager.ManualPeer(peerID)
	if err != nil {
		if ierrors.Is(err, network.ErrUnknownPeer) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "peer not found, peerID: %s", peerID.String())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get peer: %w", err)
	}

	return getPeerInfoFromPeer(peer), nil
}

// removePeer drops the connection to the peer with the given peerID and removes it from the known peers.
func removePeer(c echo.Context) error {
	peerID, err := parsePeerIDParam(c)
	if err != nil {
		return err
	}

	// error is ignored because we don't care about the config here
	_ = deps.PeeringConfigManager.RemovePeer(peerID)

	return deps.NetworkManager.RemovePeer(peerID)
}

// listPeers returns the list of all peers.
func listPeers() *api.PeersResponse {
	// get all known manual peers
	manualPeers := deps.NetworkManager.ManualPeers()

	// get all connected neighbors
	allNeighbors := deps.NetworkManager.Neighbors()

	peersMap := make(map[peer.ID]*network.Peer)
	for _, peer := range manualPeers {
		peersMap[peer.ID] = peer
	}

	for _, neighbor := range allNeighbors {
		// it's no problem if the peer is already in the map
		peersMap[neighbor.Peer().ID] = neighbor.Peer()
	}

	peers := make([]*api.PeerInfo, 0, len(peersMap))
	for _, peer := range peersMap {
		peers = append(peers, getPeerInfoFromPeer(peer))
	}

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].ID < peers[j].ID
	})

	return &api.PeersResponse{
		Peers: peers,
	}
}

// addPeer adds the peer with the given multiAddress to the manual peering layer.
func addPeer(c echo.Context) (*api.PeerInfo, error) {
	request := &api.AddPeerRequest{}

	if err := c.Bind(request); err != nil {
		return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid addPeerRequest: %w", err)
	}

	multiAddr, err := multiaddr.NewMultiaddr(request.MultiAddress)
	if err != nil {
		return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid multiAddress (%s): %w", request.MultiAddress, err)
	}

	_, err = peer.AddrInfoFromP2pAddr(multiAddr)
	if err != nil {
		return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid address info from multiAddress (%s): %w", request.MultiAddress, err)
	}

	peer, err := deps.NetworkManager.AddManualPeer(multiAddr)
	if err != nil {
		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to add peer: %w", err)
	}

	var alias string
	if request.Alias != "" {
		alias = request.Alias
	}

	// error is ignored because we don't care about the config here
	if err := deps.PeeringConfigManager.AddPeer(multiAddr, alias); err != nil {
		Component.LogWarnf("failed to add peer to config, peerID: %s, err: %s", peer.ID.String(), err.Error())
	}

	return getPeerInfoFromPeer(peer), nil
}
