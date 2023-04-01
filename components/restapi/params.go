package restapi

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersRestAPI contains the definition of the parameters used by REST API.
type ParametersRestAPI struct {
	// Enabled defines whether the REST API plugin is enabled.
	Enabled bool `default:"true" usage:"whether the REST API plugin is enabled"`
	// the bind address on which the REST API listens on
	BindAddress string `default:"0.0.0.0:14265" usage:"the bind address on which the REST API listens on"`
	// the HTTP REST routes which can be called without authorization. Wildcards using * are allowed
	PublicRoutes []string `usage:"the HTTP REST routes which can be called without authorization. Wildcards using * are allowed"`
	// the HTTP REST routes which need to be called with authorization. Wildcards using * are allowed
	ProtectedRoutes []string `usage:"the HTTP REST routes which need to be called with authorization. Wildcards using * are allowed"`
	// whether the debug logging for requests should be enabled
	DebugRequestLoggerEnabled bool `default:"false" usage:"whether the debug logging for requests should be enabled"`

	JWTAuth struct {
		// salt used inside the JWT tokens for the REST API. Change this to a different value to invalidate JWT tokens not matching this new value
		Salt string `default:"IOTA" usage:"salt used inside the JWT tokens for the REST API. Change this to a different value to invalidate JWT tokens not matching this new value"`
	} `name:"jwtAuth"`

	Limits struct {
		// the maximum number of characters that the body of an API call may contain
		MaxBodyLength string `default:"1M" usage:"the maximum number of characters that the body of an API call may contain"`
		// the maximum number of results that may be returned by an endpoint
		MaxResults int `default:"1000" usage:"the maximum number of results that may be returned by an endpoint"`
	}
}

var ParamsRestAPI = &ParametersRestAPI{
	PublicRoutes: []string{
		"/health",
		"/api/routes",
	},
	ProtectedRoutes: []string{
		"/api/*",
	},
}

var params = &app.ComponentParams{
	Params: map[string]any{
		"restAPI": ParamsRestAPI,
	},
	Masked: []string{"restAPI.jwtAuth.salt"},
}
