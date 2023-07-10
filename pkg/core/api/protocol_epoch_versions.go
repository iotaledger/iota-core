package api

import (
	"golang.org/x/exp/slices"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ProtocolEpochVersions struct {
	versionsPerEpoch []ProtocolEpochVersion
}

func NewProtocolEpochVersions() *ProtocolEpochVersions {
	return &ProtocolEpochVersions{
		versionsPerEpoch: make([]ProtocolEpochVersion, 0),
	}
}

func (p *ProtocolEpochVersions) VersionForEpoch(epoch iotago.EpochIndex) iotago.Version {
	for i := len(p.versionsPerEpoch) - 1; i >= 0; i-- {
		if p.versionsPerEpoch[i].StartEpoch <= epoch {
			return p.versionsPerEpoch[i].Version
		}
	}

	// This means that the protocol versions are not properly configured.
	panic(ierrors.Errorf("could not find a protocol version for epoch %d", epoch))
}

func (p *ProtocolEpochVersions) Add(version iotago.Version, epoch iotago.EpochIndex) {
	p.versionsPerEpoch = append(p.versionsPerEpoch, ProtocolEpochVersion{
		Version:    version,
		StartEpoch: epoch,
	})

	slices.SortFunc(p.versionsPerEpoch, func(a, b ProtocolEpochVersion) bool {
		return a.Version < b.Version
	})
}

func (p *ProtocolEpochVersions) Slice() []ProtocolEpochVersion {
	return lo.CopySlice(p.versionsPerEpoch)
}

type ProtocolEpochVersion struct {
	Version    iotago.Version
	StartEpoch iotago.EpochIndex
}
