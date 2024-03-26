package app

import (
	"fmt"
	"os"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/components/profiling"
	"github.com/iotaledger/hive.go/app/components/shutdown"
	dashboardmetrics "github.com/iotaledger/iota-core/components/dashboard_metrics"
	"github.com/iotaledger/iota-core/components/debugapi"
	"github.com/iotaledger/iota-core/components/inx"
	"github.com/iotaledger/iota-core/components/metricstracker"
	"github.com/iotaledger/iota-core/components/p2p"
	"github.com/iotaledger/iota-core/components/prometheus"
	"github.com/iotaledger/iota-core/components/protocol"
	"github.com/iotaledger/iota-core/components/restapi"
	coreapi "github.com/iotaledger/iota-core/components/restapi/core"
	"github.com/iotaledger/iota-core/components/restapi/management"
	"github.com/iotaledger/iota-core/pkg/toolset"
)

var (
	// Name of the app.
	Name = "iota-core"

	// Version of the app.
	Version = "v1.0.0-develop"
)

func App() *app.App {
	return app.New(Name, Version,
		// app.WithVersionCheck("iotaledger", "iota-core"),
		app.WithUsageText(fmt.Sprintf(`Usage of %s (%s %s):

Run '%s tools' to list all available tools.
		
Command line flags:
`, os.Args[0], Name, Version, os.Args[0])),
		app.WithInitComponent(InitComponent),
		app.WithComponents(
			shutdown.Component,
			p2p.Component,
			profiling.Component,
			restapi.Component,
			coreapi.Component,
			management.Component,
			debugapi.Component,
			metricstracker.Component,
			protocol.Component,
			dashboardmetrics.Component,
			prometheus.Component,
			inx.Component,
		),
	)
}

var InitComponent *app.InitComponent

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
		Init: initialize,
	}
}

func initialize(_ *app.App) error {
	if toolset.ShouldHandleTools() {
		toolset.HandleTools()
		// HandleTools will call os.Exit
	}

	return nil
}
