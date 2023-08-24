package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	golibp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/dig"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/autopeering/peer/service"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/libp2putil"
	"github.com/iotaledger/iota-core/pkg/network/autopeering"
	"github.com/iotaledger/iota-core/pkg/network/manualpeering"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:      "P2P",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Params:    params,
		Provide:   provide,
		Configure: configure,
		Run:       run,
	}
}

var (
	Component *app.Component
	deps      dependencies

	localAddr *net.TCPAddr
)

type dependencies struct {
	dig.In

	LocalPeer        *peer.Local
	ManualPeeringMgr *manualpeering.Manager
	AutoPeeringMgr   *autopeering.Manager
	P2PManager       *p2p.Manager
	PeerDB           *peer.DB
	Protocol         *protocol.Protocol
	PeerDBKVSTore    kvstore.KVStore `name:"peerDBKVStore"`
}

func provide(c *dig.Container) error {
	type manualPeeringDeps struct {
		dig.In

		LocalPeer  *peer.Local
		P2PManager *p2p.Manager
	}

	type autoPeeringDeps struct {
		dig.In

		Protocol   *protocol.Protocol
		P2PManager *p2p.Manager
		Host       host.Host
		PeerDB     *peer.DB
	}

	if err := c.Provide(func(deps manualPeeringDeps) *manualpeering.Manager {
		return manualpeering.NewManager(deps.P2PManager, deps.LocalPeer, Component.WorkerPool, Component.Logger())
	}); err != nil {
		return err
	}

	if err := c.Provide(func(deps autoPeeringDeps) *autopeering.Manager {
		return autopeering.NewManager(deps.Protocol.LatestAPI().ProtocolParameters().NetworkName(), deps.P2PManager, deps.Host, deps.PeerDB, Component.Logger())
	}); err != nil {
		return err
	}

	if err := c.Provide(func(lPeer *peer.Local) host.Host {
		var err error

		// resolve the bind address
		localAddr, err = net.ResolveTCPAddr("tcp", ParamsP2P.BindAddress)
		if err != nil {
			Component.LogErrorfAndExit("bind address '%s' is invalid: %s", ParamsP2P.BindAddress, err)
		}

		// announce the service
		if serviceErr := lPeer.UpdateService(service.P2PKey, localAddr.Network(), localAddr.Port); serviceErr != nil {
			Component.LogErrorfAndExit("could not update services: %s", serviceErr)
		}

		libp2pIdentity, err := libp2putil.GetLibp2pIdentity(lPeer)
		if err != nil {
			Component.LogFatalfAndExit("Could not build libp2p identity from local peer: %s", err)
		}
		libp2pHost, err := golibp2p.New(
			golibp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", localAddr.IP, localAddr.Port)),
			libp2pIdentity,
			golibp2p.NATPortMap(),
		)
		if err != nil {
			Component.LogFatalfAndExit("Couldn't create libp2p host: %s", err)
		}

		return libp2pHost
	}); err != nil {
		return err
	}

	if err := c.Provide(func(host host.Host, lPeer *peer.Local) *p2p.Manager {
		return p2p.NewManager(host, lPeer, Component.Logger())
	}); err != nil {
		return err
	}

	type peerOut struct {
		dig.Out
		Peer           *peer.Local
		PeerDB         *peer.DB
		PeerDBKVSTore  kvstore.KVStore `name:"peerDBKVStore"`
		NodePrivateKey crypto.PrivKey  `name:"nodePrivateKey"`
	}

	// instantiates a local instance.
	return c.Provide(func() peerOut {
		peerDB, peerDBKVStore, isNewDB, err := initPeerDB()
		if err != nil {
			Component.LogFatalAndExit(err)
		}

		var seed [][]byte
		cfgSeedSet := ParamsP2P.Seed != ""
		if cfgSeedSet {
			readSeed, cfgReadErr := readSeedFromCfg()
			if cfgReadErr != nil {
				Component.LogFatalAndExit(cfgReadErr)
			}
			seed = append(seed, readSeed)
		}

		if !isNewDB && cfgSeedSet && !ParamsP2P.OverwriteStoredSeed {
			seedCheckErr := checkCfgSeedAgainstDB(seed[0], peerDB)
			if seedCheckErr != nil {
				Component.LogFatalAndExit(seedCheckErr)
			}
		}

		peeringIP, err := readPeerIP()
		if err != nil {
			Component.LogFatalAndExit(err)
		}

		if !peeringIP.IsGlobalUnicast() {
			Component.LogWarnf("IP is not a global unicast address: %s", peeringIP)
		}

		// TODO: remove requirement for PeeringKey in hive.go
		services := service.New()
		services.Update(service.PeeringKey, "dummy", 0)

		local, err := peer.NewLocal(peeringIP, services, peerDB, seed...)
		if err != nil {
			Component.LogFatalAndExit("Error creating local: %s", err)
		}

		dbKey, err := peerDB.LocalPrivateKey()
		if err != nil {
			Component.LogFatalAndExit(err)
		}
		libp2pPrivateKey, err := libp2putil.ToLibp2pPrivateKey(dbKey)
		if err != nil {
			Component.LogFatalAndExit(err)
		}

		Component.LogInfof("Initialized local: %v", local)

		return peerOut{
			Peer:           local,
			PeerDB:         peerDB,
			PeerDBKVSTore:  peerDBKVStore,
			NodePrivateKey: libp2pPrivateKey,
		}
	})
}

func configure() error {
	// log the p2p events
	deps.P2PManager.Events.NeighborAdded.Hook(func(neighbor *p2p.Neighbor) {
		Component.LogInfof("Neighbor added: %s / %s", neighbor.PeerAddresses, neighbor.Identity.ID())
	}, event.WithWorkerPool(Component.WorkerPool))

	deps.P2PManager.Events.NeighborRemoved.Hook(func(neighbor *p2p.Neighbor) {
		Component.LogInfof("Neighbor removed: %s / %s", neighbor.PeerAddresses, neighbor.Identity.ID())
	}, event.WithWorkerPool(Component.WorkerPool))

	return nil
}

func run() error {
	if err := Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		deps.ManualPeeringMgr.Start()
		deps.AutoPeeringMgr.Start(ctx)
		defer func() {
			if err := deps.ManualPeeringMgr.Stop(); err != nil {
				Component.LogErrorf("Failed to stop the manager", "err", err)
			}
		}()
		//nolint:contextcheck // false positive
		addPeersFromConfigToManager(deps.ManualPeeringMgr)
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

		Component.LogInfof("started: bind-address=%s", localAddr)

		<-ctx.Done()
	}, daemon.PriorityP2P); err != nil {
		Component.LogErrorfAndExit("Failed to start as daemon: %s", err)
	}

	if err := Component.Daemon().BackgroundWorker(fmt.Sprintf("%s-PeerDB", Component.Name), func(ctx context.Context) {
		<-ctx.Done()
		prvKey, _ := deps.PeerDB.LocalPrivateKey()
		if err := deps.PeerDBKVSTore.Close(); err != nil {
			Component.LogErrorfAndExit("unable to save identity %s: %s", prvKey.Public(), err)
			return
		}
		Component.LogInfof("saved identity %s", prvKey.Public())
	}, daemon.PriorityPeerDatabase); err != nil {
		Component.LogErrorfAndExit("Failed to start as daemon: %s", err)
	}

	return nil
}

func addPeersFromConfigToManager(manualPeeringMgr *manualpeering.Manager) {
	peerAddrs, err := getPeerMultiAddrsFromConfig()
	if err != nil {
		Component.LogErrorf("Failed to get known peers from the config file, continuing without them...", "err", err)

		return
	}

	Component.LogInfof("Pass known peers list from the config file to the manager", "peers", peerAddrs)
	if err := manualPeeringMgr.AddPeers(peerAddrs...); err != nil {
		Component.LogInfof("Failed to pass known peers list from the config file to the manager", "peers", peerAddrs, "err", err)
	}
}

func getPeerMultiAddrsFromConfig() ([]ma.Multiaddr, error) {
	if ParamsPeers.KnownPeers == "" {
		return nil, nil
	}
	var peersMultiAddrStrings []string
	if err := json.Unmarshal([]byte(ParamsPeers.KnownPeers), &peersMultiAddrStrings); err != nil {
		return nil, ierrors.Wrap(err, "can't parse peers from json")
	}

	peersMultiAddr := make([]ma.Multiaddr, 0, len(peersMultiAddrStrings))
	for _, peerMultiAddrString := range peersMultiAddrStrings {
		peerMultiAddr, err := ma.NewMultiaddr(peerMultiAddrString)
		if err != nil {
			return nil, ierrors.Wrap(err, "can't parse peer multiaddr")
		}
		peersMultiAddr = append(peersMultiAddr, peerMultiAddr)
	}

	return peersMultiAddr, nil
}
