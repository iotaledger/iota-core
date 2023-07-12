package api

import (
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
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

func (p *ProtocolEpochVersions) Bytes() []byte {
	versionsBytes := make([]byte, 0)
	for _, protocolEpochVersion := range p.versionsPerEpoch {
		versionsBytes = append(versionsBytes, protocolEpochVersion.Bytes()...)
	}

	return versionsBytes
}

func (p *ProtocolEpochVersions) String() string {
	builder := stringify.NewStructBuilder("ProtocolEpochVersions")

	for i, protocolEpochVersion := range p.versionsPerEpoch {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("entry%d", i), protocolEpochVersion.String()))
	}

	return builder.String()
}

type ProtocolEpochVersion struct {
	Version    iotago.Version
	StartEpoch iotago.EpochIndex
}

func (p *ProtocolEpochVersion) Bytes() []byte {
	return append(lo.PanicOnErr(p.Version.Bytes()), lo.PanicOnErr(p.StartEpoch.Bytes())...)
}

func (p *ProtocolEpochVersion) String() string {
	return fmt.Sprintf("Version %d: Epoch %d", p.Version, p.StartEpoch)
}
