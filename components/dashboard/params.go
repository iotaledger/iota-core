package dashboard

import "github.com/iotaledger/hive.go/app"

// ParametersDashboard contains the definition of configuration parameters used by the dashboard plugin.
type ParametersDashboard struct {
	// BindAddress defines the config flag of the dashboard binding address.
	BindAddress string `default:"0.0.0.0:8081" usage:"the bind address of the dashboard"`
	BasicAuth   struct {
		// Enabled defines the config flag of the dashboard basic auth enabler.
		Enabled bool `default:"false" usage:"whether to enable HTTP basic auth"`
		// Username defines the config flag of the dashboard basic auth username.
		Username string `default:"goshimmer" usage:"HTTP basic auth username"`
		// Password defines the config flag of the dashboard basic auth password.
		Password string `default:"goshimmer" usage:"HTTP basic auth password"`
	}
	// Conflicts defines the config flag for the configs tab of the dashboard.
	Conflicts struct {
		// MaxCount defines the max number of conflicts stored on the dashboard.
		MaxCount int `default:"100" usage:"max number of conflicts stored on the dashboard"`
	}
}

var ParamsDashboard = &ParametersDashboard{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"dashboard": ParamsDashboard,
	},
}
