package presets

import (
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
)

var Base = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
	snapshotcreator.WithFilePath("snapshot.bin"),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("default", "rms"),
			iotago.WithSupplyOptions(4_600_000_000_000_000, 100, 1, 10, 100, 100, 100),
			iotago.WithTimeProviderOptions(1696841745, 10, 13),
			iotago.WithLivenessOptions(30, 30, 7, 14, 30),
			// increase/decrease threshold = fraction * slotDurationInSeconds * schedulerRate
			iotago.WithCongestionControlOptions(500, 500, 500, 800000, 500000, 100000, 1000, 100),
			iotago.WithWorkScoreOptions(25, 1, 100, 50, 10, 10, 50, 1, 10, 250),
		),
	),
	snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
		iotago.EmptyBlockID: iotago.NewEmptyCommitment(3).MustID(),
	}),
}

var Docker = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithAccounts(
		snapshotcreator.AccountDetails{ // node-1-validator
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b"))),
			Amount:               testsuite.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         testsuite.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(testsuite.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{ // node-2-validator
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064"))),
			Amount:               testsuite.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         testsuite.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(testsuite.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{ // node-3-validator
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648"))),
			Amount:               testsuite.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         testsuite.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(testsuite.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{
			/*
				inx-blockissuer

				ed25519 private key:  432c624ca3260f910df35008d5c740593b222f1e196e6cdb8cd1ad080f0d4e33997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270
				ed25519 public key:   997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270
				ed25519 address:      edc1c3a42a60a04b69c2fbb6e886414c71c464afbc7d19fb63b36a8065334ada
				bech32 address:       rms1prkursay9fs2qjmfctamd6yxg9x8r3ry47786x0mvwek4qr9xd9d5c6gkun
			*/
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270"))),
			Amount:               testsuite.MinIssuerAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			Mana:                 iotago.Mana(testsuite.MinIssuerAccountAmount),
		},
	),
	snapshotcreator.WithBasicOutputs(
		/*
			inx-faucet

			ed25519 private key:  de52b9964dda96564e9fab362ab16c2669c715c6a2a853bece8a25fc58c599755b938327ea463e0c323c0fd44f6fc1843ed94daecc6909c6043d06b7152e4737
			ed25519 public key:   5b938327ea463e0c323c0fd44f6fc1843ed94daecc6909c6043d06b7152e4737
			ed25519 address:      2f64f9d179991f50542b01e034fa043b195403875b8677efaf196b41c88803d0
			bech32 address:       rms1qqhkf7w30xv375z59vq7qd86qsa3j4qrsadcval04uvkkswg3qpaqf4hga2

			=> restricted address with mana enabled: rms1xqqz7e8e69uej86s2s4srcp5lgzrkx25qwr4hpnha7h3j66pezyq85qpqg55v3ur
		*/
		snapshotcreator.BasicOutputDetails{
			Address: lo.Return2(iotago.ParseBech32("rms1xqqz7e8e69uej86s2s4srcp5lgzrkx25qwr4hpnha7h3j66pezyq85qpqg55v3ur")),
			Amount:  1_000_000_000_000_000,
			Mana:    10_000_000,
		},
	),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("docker", "rms"),
			iotago.WithSupplyOptions(4_600_000_000_000_000, 1, 1, 10, 100, 100, 100),
			iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(30, 30, 7, 14, 30),
			// increase/decrease threshold = fraction * slotDurationInSeconds * schedulerRate
			iotago.WithCongestionControlOptions(500, 500, 500, 800000, 500000, 100000, 1000, 100),
			iotago.WithWorkScoreOptions(25, 1, 100, 50, 10, 10, 50, 1, 10, 250),
		),
	),
}

// Feature is a preset for the feature network, genesis time ~20th of July 2023.
var Feature = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
	snapshotcreator.WithAccounts(
		snapshotcreator.AccountDetails{ // node-01
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe"))),
			Amount:               testsuite.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         testsuite.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(testsuite.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{ // node-02
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6"))),
			Amount:               testsuite.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         testsuite.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(testsuite.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{ // node-03
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805"))),
			Amount:               testsuite.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0xac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         testsuite.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(testsuite.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{
			/*
				inx-blockissuer

				ed25519 public key:   670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25
				ed25519 address:      3b07e3e84c1276f0b9d35cf218b3763e0cbdadaa7ca38588de2170c31b38e9bb
				bech32 address:       rms1pqas0clgfsf8du9e6dw0yx9nwclqe0dd4f728pvgmcshpscm8r5mkddrrfc
			*/
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25"))),
			Amount:               testsuite.MinIssuerAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			Mana:                 iotago.Mana(testsuite.MinIssuerAccountAmount),
		},
	),
	snapshotcreator.WithBasicOutputs(
		/*
			inx-faucet

			ed25519 public key:   dcd760a51cfafe901f4ca0745d399af7146028af643e8a339c7bb82fbb1be7f9
			ed25519 address:      48acd764f626523646d5ccf22f807e96d30b7ab0064f370b66fa811985985ec4
			bech32 address:       rms1qpy2e4my7cn9ydjx6hx0ytuq06tdxzm6kqry7dctvmagzxv9np0vg9c55n4

			=> restricted address with mana enabled: rms1xqqy3txhvnmzv53kgm2ueu30splfd5ct02cqvnehpdn04qgeskv9a3qpqgrhlhv3
		*/
		snapshotcreator.BasicOutputDetails{
			Address: lo.Return2(iotago.ParseBech32("rms1xqqy3txhvnmzv53kgm2ueu30splfd5ct02cqvnehpdn04qgeskv9a3qpqgrhlhv3")),
			Amount:  1_000_000_000_000_000,
			Mana:    10_000_000,
		},
	),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("feature", "rms"),
			iotago.WithSupplyOptions(4_600_000_000_000_000, 100, 1, 10, 100, 100, 100),
			iotago.WithTimeProviderOptions(1697631694, 10, 13),
			iotago.WithLivenessOptions(30, 30, 10, 20, 30),
			// increase/decrease threshold = fraction * slotDurationInSeconds * schedulerRate
			iotago.WithCongestionControlOptions(500, 500, 500, 800000, 500000, 100000, 1000, 100),
			iotago.WithWorkScoreOptions(25, 1, 100, 50, 10, 10, 50, 1, 10, 250),
		),
	),
}
