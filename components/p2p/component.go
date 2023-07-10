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

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/autopeering/peer/service"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/libp2putil"
	"github.com/iotaledger/iota-core/pkg/network/manualpeering"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
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
	P2PManager       *p2p.Manager
	PeerDB           *peer.DB
	PeerDBKVSTore    kvstore.KVStore `name:"peerDBKVStore"`
}

func provide(c *dig.Container) error {
	type manualPeeringDeps struct {
		dig.In

		LocalPeer  *peer.Local
		P2PManager *p2p.Manager
	}

	if err := c.Provide(func(deps manualPeeringDeps) *manualpeering.Manager {
		return manualpeering.NewManager(deps.P2PManager, deps.LocalPeer, Component.WorkerPool, Component.Logger())
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
	deps.P2PManager.NeighborGroupEvents(p2p.NeighborsGroupAuto).NeighborAdded.Hook(func(event *p2p.NeighborAddedEvent) {
		n := event.Neighbor
		Component.LogInfof("Neighbor added: %s / %s", p2p.GetAddress(n.Peer), n.ID())
	}, event.WithWorkerPool(Component.WorkerPool))

	deps.P2PManager.NeighborGroupEvents(p2p.NeighborsGroupAuto).NeighborRemoved.Hook(func(event *p2p.NeighborRemovedEvent) {
		n := event.Neighbor
		Component.LogInfof("Neighbor removed: %s / %s", p2p.GetAddress(n.Peer), n.ID())
	}, event.WithWorkerPool(Component.WorkerPool))

	return nil
}

func run() error {
	if err := Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		deps.ManualPeeringMgr.Start()
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
		defer deps.P2PManager.Stop()
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

func addPeersFromConfigToManager(mgr *manualpeering.Manager) {
	peers, err := getKnownPeersFromConfig()
	if err != nil {
		Component.LogErrorf("Failed to get known peers from the config file, continuing without them...", "err", err)
	} else if len(peers) != 0 {
		Component.LogInfof("Pass known peers list from the config file to the manager", "peers", peers)
		if err := mgr.AddPeer(peers...); err != nil {
			Component.LogInfof("Failed to pass known peers list from the config file to the manager",
				"peers", peers, "err", err)
		}
	}
}

func getKnownPeersFromConfig() ([]*manualpeering.KnownPeerToAdd, error) {
	if ParamsPeers.KnownPeers == "" {
		return []*manualpeering.KnownPeerToAdd{}, nil
	}
	var peers []*manualpeering.KnownPeerToAdd
	if err := json.Unmarshal([]byte(ParamsPeers.KnownPeers), &peers); err != nil {
		return nil, ierrors.Wrap(err, "can't parse peers from json")
	}

	return peers, nil
}
