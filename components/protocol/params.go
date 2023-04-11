package protocol

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersProtocol
type ParametersProtocol struct {
	// Snapshot contains snapshots related configuration parameters.
	Snapshot struct {
		// Path is the path to the snapshot file.
		Path string `default:"./snapshot.bin" usage:"the path of the snapshot file"`
		// Depth defines how many slot diffs are stored in the snapshot, starting from the full ledgerstate.
		Depth int `default:"5" usage:"defines how many slot diffs are stored in the snapshot, starting from the full ledgerstate"`
	}
}

// ParametersDatabase contains the definition of configuration parameters used by the storage layer.
type ParametersDatabase struct {
	// Directory defines the directory of the database.
	Directory string `default:"db" usage:"path to the database directory"`

	// InMemory defines whether to use an in-memory database.
	InMemory bool `default:"false" usage:"whether the database is only kept in memory and not persisted"`

	MaxOpenDBs       int    `default:"10" usage:"maximum number of open database instances"`
	PruningThreshold uint64 `default:"360" usage:"how many confirmed slots should be retained"`
	DBGranularity    int64  `default:"1" usage:"how many slots should be contained in a single DB instance"`

	Settings struct {
		// Path is the path to the settings file.
		FileName string `default:"settings.bin" usage:"the file name of the settings file, relative to the database directory"`
	}
}

// ParamsProtocol
var ParamsProtocol = &ParametersProtocol{}

// ParamsDatabase contains configuration parameters used by Database.
var ParamsDatabase = &ParametersDatabase{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"protocol": ParamsProtocol,
		"database": ParamsDatabase,
	},
}
