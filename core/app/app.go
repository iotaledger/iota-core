package app

import (
	"github.com/iotaledger/hive.go/core/app"
	"github.com/iotaledger/hive.go/core/app/core/shutdown"
	"github.com/iotaledger/hive.go/core/app/plugins/profiling"
	"github.com/iotaledger/iota-core/core/p2p"
	"github.com/iotaledger/iota-core/plugins/restapi"
)

var (
	// Name of the app.
	Name = "iota-core"

	// Version of the app.
	Version = "0.1.0"
)

func App() *app.App {
	return app.New(Name, Version,
		app.WithVersionCheck("iotaledger", "iota-core"),
		app.WithInitComponent(InitComponent),
		app.WithCoreComponents([]*app.CoreComponent{
			shutdown.CoreComponent,
			p2p.CoreComponent,
		}...),
		app.WithPlugins([]*app.Plugin{
			profiling.Plugin,
			restapi.Plugin,
		}...),
	)
}

var (
	InitComponent *app.InitComponent
)

func init() {
	InitComponent = &app.InitComponent{
		Component: &app.Component{
			Name: "App",
		},
		NonHiddenFlags: []string{
			"config",
			"help",
			"peering",
			"version",
		},
		AdditionalConfigs: []*app.ConfigurationSet{
			app.NewConfigurationSet("peering", "peering", "peeringConfigFilePath", "peeringConfig", false, true, false, "peering.json", "n"),
		},
	}
}
