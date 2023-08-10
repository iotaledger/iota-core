package testsuite

import (
	"os"
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
)

func WithWaitFor(waitFor time.Duration) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsWaitFor = waitFor
	}
}

func WithTick(tick time.Duration) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsTick = tick
	}
}

func WithAccounts(accounts ...snapshotcreator.AccountDetails) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsAccounts = append(opts.optsAccounts, accounts...)
	}
}

func WithSnapshotOptions(snapshotOptions ...options.Option[snapshotcreator.Options]) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsSnapshotOptions = snapshotOptions
	}
}

func WithGenesisTimestampOffset(offset int64) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsGenesisTimestampOffset = offset
	}
}

func WithLivenessThreshold(livenessThreshold iotago.SlotIndex) options.Option[TestSuite] {
	// TODO: eventually this should not be used and common parameters should be used

	return func(opts *TestSuite) {
		opts.optsLivenessThreshold = livenessThreshold
	}
}

func WithMinCommittableAge(minCommittableAge iotago.SlotIndex) options.Option[TestSuite] {
	// TODO: eventually this should not be used and common parameters should be used

	return func(opts *TestSuite) {
		opts.optsMinCommittableAge = minCommittableAge
	}
}

func WithMaxCommittableAge(maxCommittableAge iotago.SlotIndex) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsMaxCommittableAge = maxCommittableAge
	}
}

func WithSlotsPerEpochExponent(slotsPerEpochExponent uint8) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsSlotsPerEpochExponent = slotsPerEpochExponent
	}
}

func WithEpochNearingThreshold(epochNearingThreshold iotago.SlotIndex) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsEpochNearingThreshold = epochNearingThreshold
	}
}

func DurationFromEnvOrDefault(defaultDuration time.Duration, envKey string) time.Duration {
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
