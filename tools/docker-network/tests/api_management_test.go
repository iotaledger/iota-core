//go:build dockertests

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func getContextWithTimeout(duration time.Duration) context.Context {
	//nolint:lostcancel
	ctx, _ := context.WithTimeout(context.Background(), duration)
	return ctx
}

// Test_PeerManagementAPI tests if the peer management API returns the expected results.
// 1. Run docker network.
// 2. List all peers of node 1.
// 3. Delete a peer from node 1.
// 4. List all peers of node 1 again and check if the peer was deleted.
// 5. Re-Add the peer to node 1.
// 6. List all peers of node 1 again and check if the peer was added.
func Test_PeerManagementAPI(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 10, 2, 384),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	runErr := d.Run()
	require.NoError(t, runErr)

	d.WaitUntilNetworkReady()

	// get the management client
	managementClient, err := d.Client("V1").Management(getContextWithTimeout(5 * time.Second))
	require.NoError(t, err)

	type test struct {
		name     string
		testFunc func(t *testing.T)
	}

	var removedPeerInfo *api.PeerInfo
	tests := []*test{
		{
			name: "List all peers of node 1",
			testFunc: func(t *testing.T) {
				peersResponse, err := managementClient.Peers(getContextWithTimeout(5 * time.Second))
				require.NoError(t, err)
				require.NotNil(t, peersResponse)
				require.Equal(t, 5, len(peersResponse.Peers))
			},
		},
		{
			name: "Delete a peer from node 1",
			testFunc: func(t *testing.T) {
				peersResponse, err := managementClient.Peers(getContextWithTimeout(5 * time.Second))
				require.NotNil(t, peersResponse)
				removedPeerInfo = peersResponse.Peers[0]
				require.NoError(t, err)
				require.Equal(t, 5, len(peersResponse.Peers))

				err = managementClient.RemovePeerByID(getContextWithTimeout(5*time.Second), removedPeerInfo.ID)
				require.NoError(t, err)
			},
		},
		{
			name: "List all peers of node 1 again and check if the peer was deleted",
			testFunc: func(t *testing.T) {
				peersResponse, err := managementClient.Peers(getContextWithTimeout(5 * time.Second))
				require.NoError(t, err)
				require.NotNil(t, peersResponse)
				require.Equal(t, 4, len(peersResponse.Peers))
			},
		},
		{
			name: "Re-Add the peer to node 1",
			testFunc: func(t *testing.T) {
				addedPeerInfo, err := managementClient.AddPeer(getContextWithTimeout(5*time.Second), string(removedPeerInfo.MultiAddresses[0]), removedPeerInfo.Alias)
				require.NoError(t, err)
				require.NotNil(t, addedPeerInfo)
				require.Equal(t, removedPeerInfo.ID, addedPeerInfo.ID)
			},
		},
		{
			name: "List all peers of node 1 again and check if the peer was added",
			testFunc: func(t *testing.T) {
				peersResponse, err := managementClient.Peers(getContextWithTimeout(5 * time.Second))
				require.NoError(t, err)
				require.NotNil(t, peersResponse)
				require.Equal(t, 5, len(peersResponse.Peers))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.testFunc)
	}
}

func Test_PeerManagementAPI_BadRequests(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 10, 2, 384),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	runErr := d.Run()
	require.NoError(t, runErr)

	d.WaitUntilNetworkReady()

	// get the management client
	managementClient, err := d.Client("V1").Management(getContextWithTimeout(5 * time.Second))
	require.NoError(t, err)

	type test struct {
		name     string
		testFunc func(t *testing.T)
	}

	tests := []*test{
		{
			name: "Test_AddPeer_InvalidPeerMultiAddress",
			testFunc: func(t *testing.T) {
				// invalid peer address
				addedPeerInfo, err := managementClient.AddPeer(getContextWithTimeout(5*time.Second), "")
				require.Error(t, err)
				require.Nil(t, addedPeerInfo)
			},
		},
		{
			name: "Test_RemovePeerByID_UnknownPeerID",
			testFunc: func(t *testing.T) {
				// unknown peer ID
				err := managementClient.RemovePeerByID(getContextWithTimeout(5*time.Second), "unknown-peer-id")
				require.Error(t, err)
			},
		},
		{
			name: "Test_PeerByID_UnknownPeerID",
			testFunc: func(t *testing.T) {
				// unknown peer ID
				peerInfo, err := managementClient.PeerByID(getContextWithTimeout(5*time.Second), "unknown-peer-id")
				require.Error(t, err)
				require.Nil(t, peerInfo)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.testFunc)
	}
}
