package p2p

import (
	"github.com/iotaledger/hive.go/app"
)

const (
	// CfgPeers defines the static peers this node should retain a connection to (CLI).
	CfgPeers = "peers"
)

// ParametersP2P contains the definition of configuration parameters used by the p2p plugin.
type ParametersP2P struct {
	// BindAddress defines on which multi addresses the p2p service should listen on.
	BindMultiAddresses []string `default:"/ip4/0.0.0.0/tcp/14666,/ip6/::/tcp/14666" usage:"the bind multi addresses for p2p connections"`

	ConnectionManager struct {
		// Defines the high watermark to use within the connection manager.
		HighWatermark int `default:"10" usage:"the threshold up on which connections count truncates to the lower watermark"`
		// Defines the low watermark to use within the connection manager.
		LowWatermark int `default:"5" usage:"the minimum connections count to hold after the high watermark was reached"`
	}

	// Seed defines the config flag of the autopeering private key seed.
	Seed string `usage:"private key seed used to derive the node identity; optional base58 or base64 encoded 256-bit string. Prefix with 'base58:' or 'base64', respectively"`
	// OverwriteStoredSeed defines whether the private key stored in an existing peerdb should be overwritten.
	OverwriteStoredSeed bool `default:"false" usage:"whether to overwrite the private key if an existing peerdb exists"`
	// ExternalAddress defines the config flag of the network external address.
	ExternalAddress string `default:"auto" usage:"external IP address under which the node is reachable; or 'auto' to determine it automatically"`

	// Defines the private key used to derive the node identity (optional).
	IdentityPrivateKey string `default:"" usage:"private key used to derive the node identity (optional)"`

	Database struct {
		// Defines the path to the p2p database.
		Path string `default:"testnet/p2pstore" usage:"the path to the p2p database"`
	} `name:"db"`
}

// ParametersPeers contains the definition of the parameters used by peers.
type ParametersPeers struct {
	// Defines the static peers this node should retain a connection to (CLI).
	Peers []string `default:"" usage:"the static peers this node should retain a connection to (CLI)"`
	// Defines the aliases of the static peers (must be the same length like CfgP2PPeers) (CLI).
	PeerAliases []string `default:"" usage:"the aliases of the static peers (must be the same amount like \"p2p.peers\""`
	// Defines the peers to be used as discovery for other peers (CLI).
	BootstrapPeers []string `default:"" usage:"peers to be used as discovery for other peers (CLI)"`
}

var (
	ParamsP2P   = &ParametersP2P{}
	ParamsPeers = &ParametersPeers{}
)

var params = &app.ComponentParams{
	Params: map[string]any{
		"p2p": ParamsP2P,
	},
	AdditionalParams: map[string]map[string]any{
		"peeringConfig": {
			"p2p": ParamsPeers,
		},
	},
	Masked: []string{"p2p.identityPrivateKey"},
}
