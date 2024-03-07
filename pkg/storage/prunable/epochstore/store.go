package epochstore

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type Store[V any] interface {
	RestoreLastPrunedEpoch() error
	LastAccessedEpoch() (iotago.EpochIndex, error)
	LastPrunedEpoch() (iotago.EpochIndex, bool)
	Load(epoch iotago.EpochIndex) (V, error)
	Store(epoch iotago.EpochIndex, value V) error
	Stream(consumer func(epoch iotago.EpochIndex, value V) error) error
	StreamBytes(consumer func([]byte, []byte) error) error
	DeleteEpoch(epoch iotago.EpochIndex) error
	Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) ([]iotago.EpochIndex, error)
	RollbackEpochs(epoch iotago.EpochIndex) (iotago.EpochIndex, []iotago.EpochIndex, error)
}
