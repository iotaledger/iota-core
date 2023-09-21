package restapi

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersRestAPI contains the definition of the parameters used by REST API.
type ParametersRestAPI struct {
	// Enabled defines whether the REST API plugin is enabled.
	Enabled bool `default:"true" usage:"whether the REST API plugin is enabled"`
	// the bind address on which the REST API listens on
	BindAddress string `default:"0.0.0.0:8080" usage:"the bind address on which the REST API listens on"`
	// the HTTP REST routes which can be called without authorization. Wildcards using * are allowed
	PublicRoutes []string `usage:"the HTTP REST routes which can be called without authorization. Wildcards using * are allowed"`
	// the HTTP REST routes which need to be called with authorization. Wildcards using * are allowed
	ProtectedRoutes []string `usage:"the HTTP REST routes which need to be called with authorization. Wildcards using * are allowed"`
	// whether the debug logging for requests should be enabled
	DebugRequestLoggerEnabled bool `default:"false" usage:"whether the debug logging for requests should be enabled"`
	// AllowIncompleteBlock defines whether the node allows to fill in incomplete block and issue it for user.
	AllowIncompleteBlock bool `default:"false" usage:"whether the node allows to fill in incomplete block and issue it for user"`
	// MaxPageSize defines the maximum number of results per page.
	MaxPageSize uint32 `default:"100" usage:"the maximum number of results per page"`
	// RequestsMemoryCacheGranularity defines per how many slots a cache is created for big API requests.
	RequestsMemoryCacheGranularity uint32 `default:"10" usage:"defines per how many slots a cache is created for big API requests"`
	// MaxRequestedSlotAge defines the maximum age of a request that will be processed.
	MaxRequestedSlotAge uint32 `default:"10" usage:"the maximum age of a request that will be processed"`

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

	// BlockIssuerAccount the accountID of the account that will issue the blocks.
	BlockIssuerAccount string `default:"" usage:"the accountID of the account that will issue the blocks"`

	// BlockIssuerPrivateKey the private key of the account that will issue the blocks.
	BlockIssuerPrivateKey string `default:"" usage:"the private key of the account that will issue the blocks"`
}

var ParamsRestAPI = &ParametersRestAPI{
	PublicRoutes: []string{
		"/health",
		"/api/routes",
		"/api/core/v3/info",
		"/api/core/v3/blocks*",
		"/api/core/v3/transactions*",
		"/api/core/v3/commitments*",
		"/api/core/v3/outputs*",
		"/api/core/v3/accounts*",
		"/api/core/v3/validators*",
		"/api/core/v3/rewards*",
		"/api/core/v3/committee",
		"/api/debug/v2/*",
		"/api/indexer/v2/*",
		"/api/mqtt/v2",
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
