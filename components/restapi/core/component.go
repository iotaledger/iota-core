package core

import (
	"fmt"
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
	"github.com/iotaledger/iota.go/v4/nodeclient"
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
		Component.LogPanicf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}

	formatEndpoint := func(endpoint string, parameter string) string {
		return fmt.Sprintf(endpoint, ":"+parameter)
	}

	routeGroup := deps.RestRouteManager.AddRoute(nodeclient.CorePluginName)

	routeGroup.GET(nodeclient.CoreEndpointInfo, func(c echo.Context) error {
		resp := info()

		return responseByHeader(c, resp)
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointBlock, restapipkg.ParameterBlockID), func(c echo.Context) error {
		resp, err := blockByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointBlockMetadata, restapipkg.ParameterBlockID), func(c echo.Context) error {
		resp, err := blockMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointBlockWithMetadata, restapipkg.ParameterBlockID), func(c echo.Context) error {
		resp, err := blockWithMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.POST(nodeclient.CoreEndpointBlocks, func(c echo.Context) error {
		resp, err := sendBlock(c)
		if err != nil {
			return err
		}
		c.Response().Header().Set(echo.HeaderLocation, resp.BlockID.ToHex())

		return httpserver.JSONResponse(c, http.StatusCreated, resp)
	}, checkNodeSynced())

	routeGroup.GET(nodeclient.CoreEndpointBlockIssuance, func(c echo.Context) error {
		resp, err := blockIssuance()
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointCommitmentByID, restapipkg.ParameterCommitmentID), func(c echo.Context) error {
		commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentByID(commitmentID)
		if err != nil {
			return err
		}

		return responseByHeader(c, commitment.Commitment())
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointCommitmentByIDUTXOChanges, restapipkg.ParameterCommitmentID), func(c echo.Context) error {
		commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
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

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointCommitmentByIndex, restapipkg.ParameterSlotIndex), func(c echo.Context) error {
		index, err := httpserver.ParseSlotParam(c, restapipkg.ParameterSlotIndex)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentBySlot(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, commitment.Commitment())
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointCommitmentByIndexUTXOChanges, restapipkg.ParameterSlotIndex), func(c echo.Context) error {
		slot, err := httpserver.ParseSlotParam(c, restapipkg.ParameterSlotIndex)
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

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointOutput, restapipkg.ParameterOutputID), func(c echo.Context) error {
		resp, err := outputByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointOutputMetadata, restapipkg.ParameterOutputID), func(c echo.Context) error {
		resp, err := outputMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointOutputWithMetadata, restapipkg.ParameterOutputID), func(c echo.Context) error {
		resp, err := outputWithMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointTransactionsIncludedBlock, restapipkg.ParameterTransactionID), func(c echo.Context) error {
		block, err := blockByTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, block.ProtocolBlock())
	})

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointTransactionsIncludedBlockMetadata, restapipkg.ParameterTransactionID), func(c echo.Context) error {
		resp, err := blockMetadataFromTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointCongestion, restapipkg.ParameterBech32Address), func(c echo.Context) error {
		resp, err := congestionByAccountAddress(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(nodeclient.CoreEndpointValidators, func(c echo.Context) error {
		resp, err := validators(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointValidatorsAccount, restapipkg.ParameterBech32Address), func(c echo.Context) error {
		resp, err := validatorByAccountAddress(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(formatEndpoint(nodeclient.CoreEndpointRewards, restapipkg.ParameterOutputID), func(c echo.Context) error {
		resp, err := rewardsByOutputID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(nodeclient.CoreEndpointCommittee, func(c echo.Context) error {
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
			if !deps.Protocol.MainEngineInstance().SyncManager.IsNodeSynced() {
				return ierrors.Wrap(echo.ErrServiceUnavailable, "node is not synced")
			}

			return next(c)
		}
	}
}

func responseByHeader(c echo.Context, obj any) error {
	// TODO: that should take the API that belongs to the object
	return httpserver.SendResponseByHeader(c, deps.Protocol.CommittedAPI(), obj)
}
