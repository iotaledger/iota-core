package core

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/metricstracker"
	"github.com/iotaledger/iota-core/components/protocol"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	protocolpkg "github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/api"
)

func init() {
	Component = &app.Component{
		Name:      "CoreAPIV3",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
		IsEnabled: func(c *dig.Container) bool {
			return restapi.ParamsRestAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies

	features = []string{}
)

type dependencies struct {
	dig.In

	AppInfo          *app.Info
	RestRouteManager *restapipkg.RestRouteManager
	Protocol         *protocolpkg.Protocol
	BlockHandler     *blockhandler.BlockHandler
	MetricsTracker   *metricstracker.MetricsTracker
	BaseToken        *protocol.BaseToken
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogFatalf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}

	routeGroup := deps.RestRouteManager.AddRoute(api.CorePluginName)

	routeGroup.GET(api.CoreEndpointInfo, func(c echo.Context) error {
		resp := info()

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointBlock), func(c echo.Context) error {
		resp, err := blockByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointBlockMetadata), func(c echo.Context) error {
		resp, err := blockMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointBlockWithMetadata), func(c echo.Context) error {
		resp, err := blockWithMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.POST(api.CoreEndpointBlocks, func(c echo.Context) error {
		resp, err := sendBlock(c)
		if err != nil {
			return err
		}
		c.Response().Header().Set(echo.HeaderLocation, resp.BlockID.ToHex())

		return responseByHeader(c, resp, http.StatusCreated)
	}, checkNodeSynced())

	routeGroup.GET(api.CoreEndpointBlockIssuance, func(c echo.Context) error {
		resp, err := blockIssuance()
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCommitmentByID), func(c echo.Context) error {
		commitmentID, err := httpserver.ParseCommitmentIDParam(c, api.ParameterCommitmentID)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentByID(commitmentID)
		if err != nil {
			return err
		}

		return responseByHeader(c, commitment.Commitment())
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCommitmentByIDUTXOChanges), func(c echo.Context) error {
		commitmentID, err := httpserver.ParseCommitmentIDParam(c, api.ParameterCommitmentID)
		if err != nil {
			return err
		}

		// load the commitment to check if it matches the given commitmentID
		commitment, err := getCommitmentByID(commitmentID)
		if err != nil {
			return err
		}

		resp, err := getUTXOChanges(commitment.ID())
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCommitmentByIDUTXOChangesFull), func(c echo.Context) error {
		commitmentID, err := httpserver.ParseCommitmentIDParam(c, api.ParameterCommitmentID)
		if err != nil {
			return err
		}

		// load the commitment to check if it matches the given commitmentID
		commitment, err := getCommitmentByID(commitmentID)
		if err != nil {
			return err
		}

		resp, err := getUTXOChangesFull(commitment.ID())
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCommitmentBySlot), func(c echo.Context) error {
		index, err := httpserver.ParseSlotParam(c, api.ParameterSlot)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentBySlot(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, commitment.Commitment())
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCommitmentBySlotUTXOChanges), func(c echo.Context) error {
		slot, err := httpserver.ParseSlotParam(c, api.ParameterSlot)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentBySlot(slot)
		if err != nil {
			return err
		}

		resp, err := getUTXOChanges(commitment.ID())
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCommitmentBySlotUTXOChangesFull), func(c echo.Context) error {
		slot, err := httpserver.ParseSlotParam(c, api.ParameterSlot)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentBySlot(slot)
		if err != nil {
			return err
		}

		resp, err := getUTXOChangesFull(commitment.ID())
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointOutput), func(c echo.Context) error {
		resp, err := outputByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointOutputMetadata), func(c echo.Context) error {
		resp, err := outputMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointOutputWithMetadata), func(c echo.Context) error {
		resp, err := outputWithMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointTransactionsIncludedBlock), func(c echo.Context) error {
		block, err := blockByTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, block.ProtocolBlock())
	})

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointTransactionsIncludedBlockMetadata), func(c echo.Context) error {
		resp, err := blockMetadataFromTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointTransactionsMetadata), func(c echo.Context) error {
		resp, err := transactionMetadataFromTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointCongestion), func(c echo.Context) error {
		resp, err := congestionByAccountAddress(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.CoreEndpointValidators, func(c echo.Context) error {
		resp, err := validators(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointValidatorsAccount), func(c echo.Context) error {
		resp, err := validatorByAccountAddress(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.EndpointWithEchoParameters(api.CoreEndpointRewards), func(c echo.Context) error {
		resp, err := rewardsByOutputID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(api.CoreEndpointCommittee, func(c echo.Context) error {
		resp, err := selectedCommittee(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	return nil
}

// AddFeature adds a feature to the RouteInfo endpoint.
func AddFeature(feature string) {
	features = append(features, strings.ToLower(feature))
}

func checkNodeSynced() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !deps.Protocol.Engines.Main.Get().SyncManager.IsNodeSynced() {
				return ierrors.Wrap(echo.ErrServiceUnavailable, "node is not synced")
			}

			return next(c)
		}
	}
}

func responseByHeader(c echo.Context, obj any, httpStatusCode ...int) error {
	// TODO: that should take the API that belongs to the object
	return httpserver.SendResponseByHeader(c, deps.Protocol.CommittedAPI(), obj, httpStatusCode...)
}
