package protocol

import (
	"time"

	"github.com/iotaledger/hive.go/app"
)

// ParametersProtocol contains the definition of the configuration parameters used by the Protocol.
type ParametersProtocol struct {
	// Snapshot contains snapshots related configuration parameters.
	Snapshot struct {
		// Path is the path to the snapshot file.
		Path string `default:"testnet/snapshot.bin" usage:"the path of the snapshot file"`
		// Depth defines how many slot diffs are stored in the snapshot, starting from the full ledgerstate.
		Depth int `default:"5" usage:"defines how many slot diffs are stored in the snapshot, starting from the full ledgerstate"`
	}

	Filter struct {
		// MaxAllowedClockDrift defines the maximum drift our wall clock can have to future blocks being received from the network.
		MaxAllowedClockDrift time.Duration `default:"5s" usage:"the maximum drift our wall clock can have to future blocks being received from the network"`
		MinCommittableAge    uint64        `default:"6" usage:"the minimum age of a commitment or commitment input, relative to block issuance time"`
		MaxCommittableAge    uint64        `default:"12" usage:"the maximum age of a commitment or commitment input, relative to block issuance time"`
	}

	BaseToken BaseToken
}

type BaseToken struct {
	// the base token name
	Name string `default:"Shimmer" usage:"the base token name"`
	// the base token ticker symbol
	TickerSymbol string `default:"SMR" usage:"the base token ticker symbol"`
	// the base token unit
	Unit string `default:"SMR" usage:"the base token unit"`
	// the base token subunit
	Subunit string `default:"glow" usage:"the base token subunit"`
	// the base token amount of decimals
	Decimals uint32 `default:"6" usage:"the base token amount of decimals"`
	// the base token uses the metric prefix
	UseMetricPrefix bool `default:"false" usage:"the base token uses the metric prefix"`
}

// ParametersDatabase contains the definition of configuration parameters used by the storage layer.
type ParametersDatabase struct {
	Engine           string `default:"rocksdb" usage:"the used database engine (rocksdb/mapdb)"`
	Path             string `default:"testnet/database" usage:"the path to the database folder"`
	MaxOpenDBs       int    `default:"10" usage:"maximum number of open database instances"`
	PruningThreshold uint64 `default:"16384" usage:"how many confirmed slots should be retained"`
	DBGranularity    int64  `default:"8192" usage:"how many slots should be contained in a single DB instance"`
}

// ParamsProtocol contains the configuration parameters used by the Protocol.
var ParamsProtocol = &ParametersProtocol{}

// ParamsDatabase contains configuration parameters used by Database.
var ParamsDatabase = &ParametersDatabase{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"protocol": ParamsProtocol,
		"database": ParamsDatabase,
	},
}
