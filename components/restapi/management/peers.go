package management

import (
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
	peerID, err := peer.Decode(c.Param("peerID"))
	if err != nil {
		return "", ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid peerID: %w", err)
	}

	return peerID, nil
}

// getPeerInfo returns the peer info for the given neighbor.
func getPeerInfo(neighbor network.Neighbor) *api.PeerInfo {
	peer := neighbor.Peer()

	multiAddresses := make([]iotago.PrefixedStringUint8, len(peer.PeerAddresses))
	for i, multiAddress := range peer.PeerAddresses {
		multiAddresses[i] = iotago.PrefixedStringUint8(multiAddress.String())
	}

	var alias string
	relation := PeerRelationAutopeered

	if peerConfigItem := deps.PeeringConfigManager.Peer(neighbor.Peer().ID); peerConfigItem != nil {
		alias = peerConfigItem.Alias

		// if the peer exists in the config, it is a manual peered peer
		relation = PeerRelationManual
	}

	return &api.PeerInfo{
		ID:             peer.ID.String(),
		MultiAddresses: multiAddresses,
		Alias:          alias,
		Relation:       relation,
		Connected:      peer.ConnStatus.Load() == network.ConnStatusConnected,
		GossipMetrics: &api.PeerGossipMetrics{
			PacketsReceived: uint32(neighbor.PacketsRead()),
			PacketsSent:     uint32(neighbor.PacketsWritten()),
		},
	}
}

// getPeer returns the peer info for the given peerID in the request.
func getPeer(c echo.Context) (*api.PeerInfo, error) {
	peerID, err := parsePeerIDParam(c)
	if err != nil {
		return nil, err
	}

	neighbor, err := deps.NetworkManager.Neighbor(peerID)
	if err != nil {
		if ierrors.Is(err, network.ErrUnknownPeer) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "peer not found, peerID: %s", peerID.String())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get peer: %w", err)
	}

	return getPeerInfo(neighbor), nil
}

// removePeer drops the connection to the peer with the given peerID and removes it from the known peers.
func removePeer(c echo.Context) error {
	peerID, err := parsePeerIDParam(c)
	if err != nil {
		return err
	}

	// error is ignored because we don't care about the config here
	_ = deps.PeeringConfigManager.RemovePeer(peerID)

	return deps.NetworkManager.DropNeighbor(peerID)
}

// listPeers returns the list of all peers.
func listPeers() *api.PeersResponse {
	allNeighbors := deps.NetworkManager.AllNeighbors()

	result := &api.PeersResponse{
		Peers: make([]*api.PeerInfo, len(allNeighbors)),
	}

	for i, info := range allNeighbors {
		result.Peers[i] = getPeerInfo(info)
	}

	return result
}

// addPeer tries to establish a connection to the given peer and adds it to the known peers.
func addPeer(c echo.Context) (*api.PeerInfo, error) {
	request := &api.AddPeerRequest{}

	if err := c.Bind(request); err != nil {
		return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid addPeerRequest: %w", err)
	}

	multiAddr, err := multiaddr.NewMultiaddr(request.MultiAddress)
	if err != nil {
		return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid multiAddress: %w", err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(multiAddr)
	if err != nil {
		return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid multiAddress: %w", err)
	}

	if err := deps.NetworkManager.AddManualPeers(multiAddr); err != nil {
		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to add peer: %w", err)
	}

	peerID := addrInfo.ID
	neighbor, err := deps.NetworkManager.Neighbor(peerID)
	if err != nil {
		if ierrors.Is(err, network.ErrUnknownPeer) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "peer not found, peerID: %s", peerID.String())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get peer: %w", err)
	}

	var alias string
	if request.Alias != "" {
		alias = request.Alias
	}

	// error is ignored because we don't care about the config here
	if err := deps.PeeringConfigManager.AddPeer(multiAddr, alias); err != nil {
		Component.LogWarnf("failed to add peer to config, peerID: %s, err: %s", peerID.String(), err.Error())
	}

	return getPeerInfo(neighbor), nil
}
