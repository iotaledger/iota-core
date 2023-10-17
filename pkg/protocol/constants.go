package protocol

const (
	// DatabaseVersion defines the current version of the database.
	DatabaseVersion byte = 1

	// SyncWindow defines the maximum amount of slots that a node requests on top of its latest verified commitment.
	SyncWindow = 1

	// WarpSyncOffset defines how many slots a commitment needs to be behind the latest commitment to be requested by
	// the warp sync process.
	WarpSyncOffset = 1
)
