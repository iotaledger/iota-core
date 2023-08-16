package debugapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	RouteValidators    = "/validators"
	RouteBlockMetadata = "/blocks/:" + restapipkg.ParameterBlockID + "/metadata"

	RouteChainManagerAllChainsDot      = "/all-chains"
	RouteChainManagerAllChainsRendered = "/all-chains/rendered"

	RouteCommitmentByIndexBlockIDs = "/commitments/by-index/:" + restapipkg.ParameterSlotIndex + "/blocks"

	RouteCommitmentByIndexTransactionIDs = "/commitments/by-index/:" + restapipkg.ParameterSlotIndex + "/transactions"
)

func init() {
	Component = &app.Component{
		Name:      "DebugAPIV3",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
		Params:    params,
		IsEnabled: func(c *dig.Container) bool {
			return restapi.ParamsRestAPI.Enabled && ParamsDebugAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies

	blocksPerSlot         *shrinkingmap.ShrinkingMap[iotago.SlotIndex, []*blocks.Block]
	blocksPrunableStorage *prunable.Manager
)

type dependencies struct {
	dig.In

	Protocol         *protocol.Protocol
	AppInfo          *app.Info
	RestRouteManager *restapipkg.RestRouteManager
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanic("RestAPI plugin needs to be enabled to use the DebugAPIV3 plugin")
	}

	blocksPerSlot = shrinkingmap.New[iotago.SlotIndex, []*blocks.Block]()
	blocksPrunableStorage = prunable.NewManager(database.Config{
		Engine:    hivedb.EngineRocksDB,
		Directory: ParamsDebugAPI.Path,

		Version:      1,
		PrefixHealth: []byte{0},
	}, func(err error) {
		fmt.Printf(">> DebugAPI Error: %s\n", err)
	}, prunable.WithGranularity(ParamsDebugAPI.DBGranularity), prunable.WithMaxOpenDBs(ParamsDebugAPI.MaxOpenDBs),
	)

	routeGroup := deps.RestRouteManager.AddRoute("debug/v2")

	deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
		blocksPerSlot.Set(block.ID().Index(), append(lo.Return1(blocksPerSlot.GetOrCreate(block.ID().Index(), func() []*blocks.Block {
			return make([]*blocks.Block, 0)
		})), block))
	})

	deps.Protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		if index < iotago.SlotIndex(ParamsDebugAPI.PruningThreshold) {
			return
		}

		blocksPrunableStorage.PruneUntilSlot(index - iotago.SlotIndex(ParamsDebugAPI.PruningThreshold))
	}, event.WithWorkerPool(workerpool.NewGroup("DebugAPI").CreatePool("PruneDebugAPI", 1)))

	deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(scd *notarization.SlotCommittedDetails) {
		if err := storeTransactionsPerSlot(scd); err != nil {
			fmt.Printf(">> DebugAPI Error: %s\n", err)
		}
	})

	deps.Protocol.Events.Engine.EvictionState.SlotEvicted.Hook(func(index iotago.SlotIndex) {
		blocksInSlot, exists := blocksPerSlot.Get(index)
		if !exists {
			return
		}

		for _, block := range blocksInSlot {
			if block.ProtocolBlock() == nil {
				fmt.Println("block is a root block", block.ID())
				continue
			}

			blockStore := blocksPrunableStorage.Get(block.ID().Index(), []byte{1})

			err := blockStore.Set(lo.PanicOnErr(block.ID().Bytes()), lo.PanicOnErr(json.Marshal(BlockMetadataResponseFromBlock(block))))
			if err != nil {
				panic(err)
			}
		}

		blocksPerSlot.Delete(index)
	})

	routeGroup.GET(RouteBlockMetadata, func(c echo.Context) error {
		blockID, err := httpserver.ParseBlockIDParam(c, restapipkg.ParameterBlockID)
		if err != nil {
			return err
		}

		if block, exists := deps.Protocol.MainEngineInstance().BlockCache.Block(blockID); exists && block.ProtocolBlock() != nil {
			response := BlockMetadataResponseFromBlock(block)

			return httpserver.JSONResponse(c, http.StatusOK, response)

		}

		blockStore := blocksPrunableStorage.Get(blockID.Index(), []byte{1})

		blockJSON, err := blockStore.Get(lo.PanicOnErr(blockID.Bytes()))
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, blockJSON)
	})

	routeGroup.GET(RouteValidators, func(c echo.Context) error {
		resp, err := validatorsSummary()
		if err != nil {
			return err
		}

		j, err := deps.Protocol.CurrentAPI().JSONEncode(resp)
		if err != nil {
			return err
		}

		return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, j)
	})

	routeGroup.GET(RouteChainManagerAllChainsDot, func(c echo.Context) error {
		resp, err := chainManagerAllChainsDot()
		if err != nil {
			return err
		}

		return c.String(http.StatusOK, resp)
	})

	routeGroup.GET(RouteChainManagerAllChainsRendered, func(c echo.Context) error {
		renderedBytes, err := chainManagerAllChainsRendered()
		if err != nil {
			return err
		}

		return c.Blob(http.StatusOK, "image/png", renderedBytes)
	})

	routeGroup.GET(RouteCommitmentByIndexBlockIDs, func(c echo.Context) error {
		slotIndex, err := httpserver.ParseSlotParam(c, restapipkg.ParameterSlotIndex)
		if err != nil {
			return err
		}

		resp, err := getSlotBlockIDs(slotIndex)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.GET(RouteCommitmentByIndexTransactionIDs, func(c echo.Context) error {
		slotIndex, err := httpserver.ParseSlotParam(c, restapipkg.ParameterSlotIndex)
		if err != nil {
			return err
		}

		resp, err := getSlotTransactionIDs(slotIndex)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}
