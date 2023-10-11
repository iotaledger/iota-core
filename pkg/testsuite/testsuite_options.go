package testsuite

import (
	"os"
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
)

func WithWaitFor(waitFor time.Duration) options.Option[TestSuite] {
	return func(t *TestSuite) {
		t.optsWaitFor = waitFor
	}
}

func WithTick(tick time.Duration) options.Option[TestSuite] {
	return func(t *TestSuite) {
		t.optsTick = tick
	}
}

func WithAccounts(accounts ...snapshotcreator.AccountDetails) options.Option[TestSuite] {
	return func(t *TestSuite) {
		t.optsAccounts = append(t.optsAccounts, accounts...)
	}
}

func WithSnapshotOptions(snapshotOptions ...options.Option[snapshotcreator.Options]) options.Option[TestSuite] {
	return func(t *TestSuite) {
		t.optsSnapshotOptions = snapshotOptions
	}
}

func WithProtocolParametersOptions(protocolParameterOptions ...options.Option[iotago.V3ProtocolParameters]) options.Option[TestSuite] {
	return func(t *TestSuite) {
		t.optsProtocolParameterOptions = protocolParameterOptions
	}
}

func GenesisTimeWithOffsetBySlots(slots iotago.SlotIndex, slotDurationInSeconds uint8) int64 {
	return time.Now().Truncate(time.Duration(slotDurationInSeconds)*time.Second).Unix() - int64(slotDurationInSeconds)*int64(slots)
}

func durationFromEnvOrDefault(defaultDuration time.Duration, envKey string) time.Duration {
	waitFor := os.Getenv(envKey)
	if waitFor == "" {
		return defaultDuration
	}

	d, err := time.ParseDuration(waitFor)
	if err != nil {
		panic(err)
	}

	return d
}

const (
	DefaultSlotDurationInSeconds uint8 = 10
	DefaultSlotsPerEpochExponent uint8 = 5

	DefaultLivenessThresholdLowerBoundInSeconds uint16           = 30
	DefaultLivenessThresholdUpperBoundInSeconds uint16           = 30
	DefaultMinCommittableAge                    iotago.SlotIndex = 10
	DefaultMaxCommittableAge                    iotago.SlotIndex = 20
	DefaultEpochNearingThreshold                iotago.SlotIndex = 16

	DefaultMinReferenceManaCost iotago.Mana      = 500
	DefaultRMCIncrease          iotago.Mana      = 500
	DefaultRMCDecrease          iotago.Mana      = 500
	DefaultRMCIncreaseThreshold iotago.WorkScore = 8 * DefaultSchedulerRate
	DefaultRMCDecreaseThreshold iotago.WorkScore = 5 * DefaultSchedulerRate
	DefaultSchedulerRate        iotago.WorkScore = 100000
	DefaultMaxBufferSize        uint32           = 100 * iotago.MaxBlockSize
	DefaultMaxValBufferSize     uint32           = 100 * iotago.MaxBlockSize
)
