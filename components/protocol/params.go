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

	Notarization struct {
		// MinSlotCommittableAge defines the min age of a committable slot.
		MinSlotCommittableAge int64 `default:"6" usage:"min age of a committable slot denoted in slots"`
	}

	Filter struct {
		// MaxAllowedClockDrift defines the maximum drift our wall clock can have to future blocks being received from the network.
		MaxAllowedClockDrift time.Duration `default:"5s" usage:"the maximum drift our wall clock can have to future blocks being received from the network"`
	}

	SybilProtection struct {
		Committee Validators `noflag:"true"`
	}
}

type Validator struct {
	Identity string `usage:"the identity of the validator"`
	Weight   int64  `usage:"the weight of the validator"`
}

type Validators []*Validator

// ParametersDatabase contains the definition of configuration parameters used by the storage layer.
type ParametersDatabase struct {
	Engine           string `default:"rocksdb" usage:"the used database engine (rocksdb/mapdb)"`
	Path             string `default:"testnet/database" usage:"the path to the database folder"`
	MaxOpenDBs       int    `default:"10" usage:"maximum number of open database instances"`
	PruningThreshold uint64 `default:"360" usage:"how many confirmed slots should be retained"`
	DBGranularity    int64  `default:"1" usage:"how many slots should be contained in a single DB instance"`
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
