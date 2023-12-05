package tests

import (
	"time"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
	"golang.org/x/crypto/blake2b"
)

// DefaultProtocolParameterOptions are the default protocol parameters for the docker network.
var DefaultProtocolParameterOptions = []options.Option[iotago.V3ProtocolParameters]{
	iotago.WithNetworkOptions("docker", "rms"),
	iotago.WithSupplyOptions(4_600_000_000_000_000, 250, 1, 1000, 100000, 500000, 100000),
	iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 13),
	iotago.WithLivenessOptions(30, 30, 7, 14, 30),
	// increase/decrease threshold = fraction * slotDurationInSeconds * schedulerRate
	iotago.WithCongestionControlOptions(500, 500, 500, 800000, 500000, 100000, 1000, 100),
	iotago.WithWorkScoreOptions(25, 1, 100, 50, 10, 10, 50, 1, 10, 250),
}

// DefaultSnapshotOptions are the default snapshot options for the docker network.
func DefaultAccountOptions(protocolParams *iotago.V3ProtocolParameters) []options.Option[snapshotcreator.Options] {
	return []options.Option[snapshotcreator.Options]{
		snapshotcreator.WithAccounts(
			snapshotcreator.AccountDetails{
				/*
					node-01-validator

					Ed25519 Public Key:   293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b
					Ed25519 Address:      rms1qzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3ytgk0ny
					Account Address:      rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6
					Restricted Address:   rms1xqqfqlqzayczurc9w8cslzz4jnjkmrz5lurs32m68x7pkaxnj6unkyspqg8mulpm, Capabilities: mana
				*/
				AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
				Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
				Amount:               mock.MinValidatorAccountAmount(protocolParams),
				IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b")))),
				ExpirySlot:           iotago.MaxSlotIndex,
				BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 5,
				StakingEndEpoch:      iotago.MaxEpochIndex,
				FixedCost:            1,
				StakedAmount:         mock.MinValidatorAccountAmount(protocolParams),
				Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(protocolParams)),
			},
			snapshotcreator.AccountDetails{
				/*
					node-02-validator

					Ed25519 Public Key:   05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064
					Ed25519 Address:      rms1qqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875flpz2p
					Account Address:      rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl
					Restricted Address:   rms1xqqrw56clykvj36sv62e3v9254dxlaenzzuswy8plt2jfs8ezxlql6spqgkulf7u, Capabilities: mana
				*/
				AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
				Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
				Amount:               mock.MinValidatorAccountAmount(protocolParams),
				IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064")))),
				ExpirySlot:           iotago.MaxSlotIndex,
				BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 5,
				StakingEndEpoch:      iotago.MaxEpochIndex,
				FixedCost:            1,
				StakedAmount:         mock.MinValidatorAccountAmount(protocolParams),
				Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(protocolParams)),
			},
			snapshotcreator.AccountDetails{
				/*
					node-03-validator

					Ed25519 Public Key:   1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648
					Ed25519 Address:      rms1qp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kuvz0a44
					Account Address:      rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt
					Restricted Address:   rms1xqqx4mnsfuj4tr525a3slmgpy8d9xp6p3z4ugganckqslq97fymwkmspqgnzrkjq, Capabilities: mana
				*/
				AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
				Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
				Amount:               mock.MinValidatorAccountAmount(protocolParams),
				IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648")))),
				ExpirySlot:           iotago.MaxSlotIndex,
				BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 5,
				StakingEndEpoch:      iotago.MaxEpochIndex,
				FixedCost:            1,
				StakedAmount:         mock.MinValidatorAccountAmount(protocolParams),
				Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(protocolParams)),
			},
			snapshotcreator.AccountDetails{
				/*
					node-04-validator

					Ed25519 Public Key:   c9ceac37d293155a578381aa313ee74edfa3ac73ee930d045564aae7771e8ffe
					Private Key: 5cceed8ca18146639330177ab4f61ab1a71e2d3fea3d4389f9e2e43f34ec8b33c9ceac37d293155a578381aa313ee74edfa3ac73ee930d045564aae7771e8ffe
					Account Address:      rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw
					Ed25519 Address:      rms1qr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvts0unx0s
					Restricted Address:   rms1xqyvnn4vxlffx92627pcr2338mn5ahar43e7aycdq32kf2h8wu0gllspqgz9eyua, Capabilities: mana
				*/
				AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xc9ceac37d293155a578381aa313ee74edfa3ac73ee930d045564aae7771e8ffe"))),
				Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xc9ceac37d293155a578381aa313ee74edfa3ac73ee930d045564aae7771e8ffe"))),
				Amount:               mock.MinValidatorAccountAmount(protocolParams),
				IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0xc9ceac37d293155a578381aa313ee74edfa3ac73ee930d045564aae7771e8ffe")))),
				ExpirySlot:           iotago.MaxSlotIndex,
				BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 5,
				StakingEndEpoch:      iotago.MaxEpochIndex,
				FixedCost:            1,
				StakedAmount:         mock.MinValidatorAccountAmount(protocolParams),
				Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(protocolParams)),
			},
			snapshotcreator.AccountDetails{
				/*
					inx-blockissuer

					Ed25519 Private Key:  432c624ca3260f910df35008d5c740593b222f1e196e6cdb8cd1ad080f0d4e33997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270
					Ed25519 Public Key:   997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270
					Ed25519 Address:      rms1qrkursay9fs2qjmfctamd6yxg9x8r3ry47786x0mvwek4qr9xd9d583cnpd
					Account Address:      rms1prkursay9fs2qjmfctamd6yxg9x8r3ry47786x0mvwek4qr9xd9d5c6gkun
					Restricted Address:   rms1xqqwmswr5s4xpgztd8p0hdhgseq5cuwyvjhmclgeld3mx65qv5e54kspqgda0nrn, Capabilities: mana
				*/
				AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270"))),
				Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270"))),
				Amount:               mock.MinIssuerAccountAmount(protocolParams),
				IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270")))),
				ExpirySlot:           iotago.MaxSlotIndex,
				BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 5,
				Mana:                 iotago.Mana(mock.MinIssuerAccountAmount(protocolParams)),
			},
		),
		snapshotcreator.WithBasicOutputs(
			/*
				inx-faucet

				Ed25519 Private Key:  de52b9964dda96564e9fab362ab16c2669c715c6a2a853bece8a25fc58c599755b938327ea463e0c323c0fd44f6fc1843ed94daecc6909c6043d06b7152e4737
				Ed25519 Public Key:   5b938327ea463e0c323c0fd44f6fc1843ed94daecc6909c6043d06b7152e4737
				Ed25519 Address:      rms1qqhkf7w30xv375z59vq7qd86qsa3j4qrsadcval04uvkkswg3qpaqf4hga2
				Restricted Address:   rms1xqqz7e8e69uej86s2s4srcp5lgzrkx25qwr4hpnha7h3j66pezyq85qpqg55v3ur, Capabilities: mana
			*/
			snapshotcreator.BasicOutputDetails{
				Address: lo.Return2(iotago.ParseBech32("rms1xqqz7e8e69uej86s2s4srcp5lgzrkx25qwr4hpnha7h3j66pezyq85qpqg55v3ur")),
				Amount:  1_000_000_000_000_000,
				Mana:    10_000_000,
			},
		)}
}

func WithProtocolParametersOptions(protocolParameterOptions ...options.Option[iotago.V3ProtocolParameters]) options.Option[DockerTestFramework] {
	return func(d *DockerTestFramework) {
		d.optsProtocolParameterOptions = protocolParameterOptions
	}
}

func WithSnapshotOptions(snapshotOptions ...options.Option[snapshotcreator.Options]) options.Option[DockerTestFramework] {
	return func(d *DockerTestFramework) {
		d.optsSnapshotOptions = snapshotOptions
	}
}

func WithWaitForSync(waitForSync time.Duration) options.Option[DockerTestFramework] {
	return func(d *DockerTestFramework) {
		d.optsWaitForSync = waitForSync
	}
}

func WithWaitFor(waitFor time.Duration) options.Option[DockerTestFramework] {
	return func(d *DockerTestFramework) {
		d.optsWaitFor = waitFor
	}
}

func WithTick(tick time.Duration) options.Option[DockerTestFramework] {
	return func(d *DockerTestFramework) {
		d.optsTick = tick
	}
}
