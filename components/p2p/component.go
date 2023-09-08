package p2p

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/configuration"
	hivep2p "github.com/iotaledger/hive.go/crypto/p2p"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/autopeering"
	"github.com/iotaledger/iota-core/pkg/network/manualpeering"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:             "P2P",
		DepsFunc:         func(cDeps dependencies) { deps = cDeps },
		Params:           params,
		InitConfigParams: initConfigParams,
		Provide:          provide,
		Configure:        configure,
		Run:              run,
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In
	PeeringConfig        *configuration.Configuration `name:"peeringConfig"`
	PeeringConfigManager *p2p.ConfigManager
	ManualPeeringMgr     *manualpeering.Manager
	AutoPeeringMgr       *autopeering.Manager
	P2PManager           *p2p.Manager
	PeerDB               *network.DB
	Protocol             *protocol.Protocol
	PeerDBKVSTore        kvstore.KVStore `name:"peerDBKVStore"`
}

func initConfigParams(c *dig.Container) error {

	type cfgResult struct {
		dig.Out
		P2PDatabasePath       string   `name:"p2pDatabasePath"`
		P2PBindMultiAddresses []string `name:"p2pBindMultiAddresses"`
	}

	if err := c.Provide(func() cfgResult {
		return cfgResult{
			P2PDatabasePath:       ParamsP2P.Database.Path,
			P2PBindMultiAddresses: ParamsP2P.BindMultiAddresses,
		}
	}); err != nil {
		Component.LogPanic(err)
	}

	return nil
}

func provide(c *dig.Container) error {
	type manualPeeringDeps struct {
		dig.In

		P2PManager *p2p.Manager
	}

	if err := c.Provide(func(deps manualPeeringDeps) *manualpeering.Manager {
		return manualpeering.NewManager(deps.P2PManager, Component.WorkerPool, Component.Logger())
	}); err != nil {
		return err
	}

	type autoPeeringDeps struct {
		dig.In

		Protocol   *protocol.Protocol
		P2PManager *p2p.Manager
		Host       host.Host
		PeerDB     *network.DB
	}

	if err := c.Provide(func(deps autoPeeringDeps) *autopeering.Manager {
		peersMultiAddresses, err := getMultiAddrsFromString(ParamsPeers.BootstrapPeers)
		if err != nil {
			Component.LogErrorfAndExit("Failed to parse bootstrapPeers param: %s", err)
		}

		for _, multiAddr := range peersMultiAddresses {
			bootstrapPeer, err := network.NewPeerFromMultiAddr(multiAddr)
			if err != nil {
				Component.LogErrorfAndExit("Failed to parse bootstrap peer multiaddress: %s", err)
			}

			if err := deps.PeerDB.UpdatePeer(bootstrapPeer); err != nil {
				Component.LogErrorf("Failed to update bootstrap peer: %s", err)
			}
		}

		return autopeering.NewManager(deps.Protocol.LatestAPI().ProtocolParameters().NetworkName(), deps.P2PManager, deps.Host, deps.PeerDB, Component.Logger())
	}); err != nil {
		return err
	}

	type peerDatabaseResult struct {
		dig.Out

		PeerDB        *network.DB
		PeerDBKVSTore kvstore.KVStore `name:"peerDBKVStore"`
	}

	if err := c.Provide(func() peerDatabaseResult {
		peerDB, peerDBKVStore, err := initPeerDB()
		if err != nil {
			Component.LogFatalAndExit(err)
		}

		return peerDatabaseResult{
			PeerDB:        peerDB,
			PeerDBKVSTore: peerDBKVStore,
		}
	}); err != nil {
		return err
	}

	type configManagerDeps struct {
		dig.In
		PeeringConfig         *configuration.Configuration `name:"peeringConfig"`
		PeeringConfigFilePath *string                      `name:"peeringConfigFilePath"`
	}

	if err := c.Provide(func(deps configManagerDeps) *p2p.ConfigManager {

		p2pConfigManager := p2p.NewConfigManager(func(peers []*p2p.PeerConfigItem) error {
			if err := deps.PeeringConfig.Set(CfgPeers, peers); err != nil {
				return err
			}

			return deps.PeeringConfig.StoreFile(*deps.PeeringConfigFilePath, 0o600, []string{"p2p"})
		})

		// peers from peering config
		var peers []*p2p.PeerConfig
		if err := deps.PeeringConfig.Unmarshal(CfgPeers, &peers); err != nil {
			Component.LogPanicf("invalid peer config: %s", err)
		}

		for i, p := range peers {
			multiAddr, err := multiaddr.NewMultiaddr(p.MultiAddress)
			if err != nil {
				Component.LogPanicf("invalid config peer address at pos %d: %s", i, err)
			}

			if err = p2pConfigManager.AddPeer(multiAddr, p.Alias); err != nil {
				Component.LogWarnf("unable to add peer to config manager %s: %s", p.MultiAddress, err)
			}
		}

		// peers from CLI arguments
		applyAliases := true
		if len(ParamsPeers.Peers) != len(ParamsPeers.PeerAliases) {
			Component.LogWarnf("won't apply peer aliases: you must define aliases for all defined static peers (got %d aliases, %d peers).", len(ParamsPeers.PeerAliases), len(ParamsPeers.Peers))
			applyAliases = false
		}

		peersMultiAddresses, err := getMultiAddrsFromString(ParamsPeers.Peers)
		if err != nil {
			Component.LogErrorAndExit(err)
		}

		peerAdded := false
		for i, multiAddr := range peersMultiAddresses {
			var alias string
			if applyAliases {
				alias = ParamsPeers.PeerAliases[i]
			}

			if err = p2pConfigManager.AddPeer(multiAddr, alias); err != nil {
				Component.LogWarnf("unable to add peer to config manager %s: %s", multiAddr.String(), err)
			}

			peerAdded = true
		}

		p2pConfigManager.StoreOnChange(true)

		if peerAdded {
			if err := p2pConfigManager.Store(); err != nil {
				Component.LogWarnf("failed to store peering config: %s", err)
			}
		}

		return p2pConfigManager
	}); err != nil {
		Component.LogPanic(err)
	}

	type p2pDeps struct {
		dig.In
		DatabaseEngine        hivedb.Engine `name:"databaseEngine"`
		P2PDatabasePath       string        `name:"p2pDatabasePath"`
		P2PBindMultiAddresses []string      `name:"p2pBindMultiAddresses"`
	}

	type p2pResult struct {
		dig.Out
		NodePrivateKey crypto.PrivKey `name:"nodePrivateKey"`
		Host           host.Host
	}

	if err := c.Provide(func(deps p2pDeps) p2pResult {

		res := p2pResult{}

		privKeyFilePath := filepath.Join(deps.P2PDatabasePath, "identity.key")

		// make sure nobody copies around the peer store since it contains the private key of the node
		Component.LogInfof(`WARNING: never share your "%s" folder as it contains your node's private key!`, deps.P2PDatabasePath)

		// load up the previously generated identity or create a new one
		nodePrivateKey, newlyCreated, err := hivep2p.LoadOrCreateIdentityPrivateKey(privKeyFilePath, ParamsP2P.IdentityPrivateKey)
		if err != nil {
			Component.LogPanic(err)
		}
		res.NodePrivateKey = nodePrivateKey

		if newlyCreated {
			Component.LogInfof(`stored new private key for peer identity under "%s"`, privKeyFilePath)
		} else {
			Component.LogInfof(`loaded existing private key for peer identity from "%s"`, privKeyFilePath)
		}

		peeringIP, err := readPeerIP()
		if err != nil {
			Component.LogFatalAndExit(err)
		}

		if !peeringIP.IsGlobalUnicast() {
			Component.LogWarnf("IP is not a global unicast address: %s", peeringIP)
		}

		connManager, err := connmgr.NewConnManager(
			ParamsP2P.ConnectionManager.LowWatermark,
			ParamsP2P.ConnectionManager.HighWatermark,
			connmgr.WithGracePeriod(time.Minute),
		)
		if err != nil {
			Component.LogPanicf("unable to initialize connection manager: %s", err)
		}

		// also see "defaults.go" in go-libp2p for the default values
		createdHost, err := libp2p.New(
			libp2p.ListenAddrStrings(ParamsP2P.BindMultiAddresses...),
			libp2p.Identity(nodePrivateKey),
			libp2p.Transport(tcp.NewTCPTransport),
			libp2p.ConnectionManager(connManager),
			libp2p.NATPortMap(),
		)
		if err != nil {
			Component.LogFatalfAndExit("unable to initialize libp2p host: %s", err)
		}
		res.Host = createdHost

		Component.LogInfof("Initialized P2P host %s %s", createdHost.ID().String(), createdHost.Addrs())

		return res
	}); err != nil {
		Component.LogPanic(err)
	}

	return c.Provide(func(host host.Host, peerDB *network.DB) *p2p.Manager {
		return p2p.NewManager(host, peerDB, Component.Logger())
	})

}

func configure() error {
	if err := Component.Daemon().BackgroundWorker("Close p2p peer database", func(ctx context.Context) {
		<-ctx.Done()

		closeDatabases := func() error {
			if err := deps.PeerDBKVSTore.Flush(); err != nil {
				return err
			}

			return deps.PeerDBKVSTore.Close()
		}

		Component.LogInfo("Syncing p2p peer database to disk ...")
		if err := closeDatabases(); err != nil {
			Component.LogPanicf("Syncing p2p peer database to disk ... failed: %s", err)
		}
		Component.LogInfo("Syncing p2p peer database to disk ... done")
	}, daemon.PriorityCloseDatabase); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	// log the p2p events
	deps.P2PManager.Events.NeighborAdded.Hook(func(neighbor *p2p.Neighbor) {
		Component.LogInfof("Neighbor added: %s / %s", neighbor.PeerAddresses, neighbor.ID)
	}, event.WithWorkerPool(Component.WorkerPool))

	deps.P2PManager.Events.NeighborRemoved.Hook(func(neighbor *p2p.Neighbor) {
		Component.LogInfof("Neighbor removed: %s / %s", neighbor.PeerAddresses, neighbor.ID)
	}, event.WithWorkerPool(Component.WorkerPool))

	return nil
}

func run() error {
	if err := Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		deps.ManualPeeringMgr.Start()
		if err := deps.AutoPeeringMgr.Start(ctx); err != nil {
			Component.LogErrorAndExit("Failed to start autopeering manager: %s", err)
		}

		defer func() {
			if err := deps.ManualPeeringMgr.Stop(); err != nil {
				Component.LogErrorf("Failed to stop the manager", "err", err)
			}
		}()
		//nolint:contextcheck // false positive
		connectConfigKnownPeers()
		<-ctx.Done()
	}, daemon.PriorityManualPeering); err != nil {
		Component.LogErrorfAndExit("Failed to start as daemon: %s", err)
	}

	if err := Component.Daemon().BackgroundWorker(fmt.Sprintf("%s-P2PManager", Component.Name), func(ctx context.Context) {
		defer deps.P2PManager.Shutdown()
		defer func() {
			if err := deps.P2PManager.P2PHost().Close(); err != nil {
				Component.LogWarn("Failed to close libp2p host: %+v", err)
			}
		}()

		<-ctx.Done()
	}, daemon.PriorityP2P); err != nil {
		Component.LogErrorfAndExit("Failed to start as daemon: %s", err)
	}

	return nil
}

func getMultiAddrsFromString(peers []string) ([]multiaddr.Multiaddr, error) {
	peersMultiAddresses := make([]multiaddr.Multiaddr, 0, len(peers))

	for _, peer := range peers {
		peerMultiAddr, err := multiaddr.NewMultiaddr(peer)
		if err != nil {
			return nil, ierrors.Errorf("invalid peer multiaddr \"%s\": %w", peer, err)
		}
		peersMultiAddresses = append(peersMultiAddresses, peerMultiAddr)
	}

	return peersMultiAddresses, nil
}

// connects to the peers defined in the config.
func connectConfigKnownPeers() {
	for _, p := range deps.PeeringConfigManager.Peers() {
		multiAddr, err := multiaddr.NewMultiaddr(p.MultiAddress)
		if err != nil {
			Component.LogPanicf("invalid peer address: %s", err)
		}

		// we try to parse the multi address and check if there is a "/p2p" part with ID
		_, err = peer.AddrInfoFromP2pAddr(multiAddr)
		if err != nil {
			Component.LogPanicf("invalid peer address info: %s", err)
		}

		if err := deps.ManualPeeringMgr.AddPeers(multiAddr); err != nil {
			Component.LogInfof("failed to add peer: %s, error: %s", multiAddr.String(), err)
		}
	}
}
