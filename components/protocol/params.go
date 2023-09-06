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
	}

	ProtocolParametersPath string `default:"testnet/protocol_parameters.json" usage:"the path of the protocol parameters file"`

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
	MaxOpenDBs       int    `default:"5" usage:"maximum number of open database instances"`
	PruningThreshold uint64 `default:"30" usage:"how many finalized epochs should be retained"`

	Size struct {
		// Enabled defines whether to delete old block data from the database based on maximum database size
		Enabled bool `default:"true" usage:"whether to delete old block data from the database based on maximum database size"`
		// TargetSize defines the target size of the database
		TargetSize string `default:"30GB" usage:"target size of the database"`
		// ReductionPercentage defines the percentage the database size gets reduced if the target size is reached
		ReductionPercentage float64 `default:"10.0" usage:"the percentage the database size gets reduced if the target size is reached"`
		// CooldownTime defines the cooldown time between two pruning by database size events
		CooldownTime time.Duration `default:"5m" usage:"cooldown time between two pruning by database size events"`
	}
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
