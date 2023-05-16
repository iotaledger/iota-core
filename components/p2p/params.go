package p2p

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersP2P contains the definition of configuration parameters used by the p2p plugin.
type ParametersP2P struct {
	// BindAddress defines on which address the p2p service should listen.
	BindAddress string `default:"0.0.0.0:14666" usage:"the bind address for p2p connections"`
	// Seed defines the config flag of the autopeering private key seed.
	Seed string `usage:"private key seed used to derive the node identity; optional base58 or base64 encoded 256-bit string. Prefix with 'base58:' or 'base64', respectively"`
	// OverwriteStoredSeed defines whether the private key stored in an existing peerdb should be overwritten.
	OverwriteStoredSeed bool `default:"false" usage:"whether to overwrite the private key if an existing peerdb exists"`
	// ExternalAddress defines the config flag of the network external address.
	ExternalAddress string `default:"auto" usage:"external IP address under which the node is reachable; or 'auto' to determine it automatically"`
	// PeerDBDirectory defines the path to the peer database.
	PeerDBDirectory string `default:"testnet/peerdb" usage:"path to the peer database directory"`
}

// ParametersPeers contains the definition of the parameters used by the manualPeering plugin.
type ParametersPeers struct {
	// KnownPeers defines the map of peers to be used as known peers.
	KnownPeers string `usage:"map of peers that will be used as known peers"`
}

// ParamsP2P contains the configuration used by the manualPeering plugin.
var ParamsP2P = &ParametersP2P{}
var ParamsPeers = &ParametersPeers{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"p2p": ParamsP2P,
	},
	AdditionalParams: map[string]map[string]any{
		"peeringConfig": {
			"p2p": ParamsPeers,
		},
	},
}
