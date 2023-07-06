package blockissuer

import (
	"context"
	"fmt"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/crypto"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol"
	iotago "github.com/iotaledger/iota.go/v4"
)

func init() {
	Component = &app.Component{
		Name:     "BlockIssuer",
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Params:   params,
		Provide:  provide,
		Run:      run,
		IsEnabled: func(_ *dig.Container) bool {
			return ParamsBlockIssuer.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	BlockIssuer *blockfactory.BlockIssuer
}

func accountFromParam(accountHex, privateKey string) blockfactory.Account {
	accountID, err := iotago.IdentifierFromHexString(accountHex)
	if err != nil {
		panic(fmt.Sprintln("invalid accountID hex string", err))
	}
	privKey, err := crypto.ParseEd25519PrivateKeyFromString(privateKey)
	if err != nil {
		panic(fmt.Sprintln("invalid ed25519 private key string", err))
	}

	return blockfactory.NewEd25519Account(accountID, privKey)
}

func provide(c *dig.Container) error {
	type innerDependencies struct {
		dig.In

		Protocol *protocol.Protocol
	}

	return c.Provide(func(deps innerDependencies) *blockfactory.BlockIssuer {
		return blockfactory.New(deps.Protocol, accountFromParam(ParamsBlockIssuer.IssuerAccount, ParamsBlockIssuer.PrivateKey),
			blockfactory.WithTipSelectionTimeout(ParamsBlockIssuer.TipSelectionTimeout),
			blockfactory.WithTipSelectionRetryInterval(ParamsBlockIssuer.TipSelectionRetryInterval),
			blockfactory.WithIncompleteBlockAccepted(restapi.ParamsRestAPI.AllowIncompleteBlock),
		)
	})
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		Component.LogInfof("Starting BlockIssuer with AccountID: %s", deps.BlockIssuer.Account.ID())
		<-ctx.Done()
		deps.BlockIssuer.Shutdown()
		Component.LogInfo("Stopping BlockIssuer... done")
	}, daemon.PriorityBlockIssuer)
}
