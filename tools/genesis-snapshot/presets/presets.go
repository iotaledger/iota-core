package presets

import (
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
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
			iotago.WithTimeProviderOptions(0, 1696841745, 10, 13),
			iotago.WithLivenessOptions(30, 30, 7, 14, 30),
			// increase/decrease threshold = fraction * slotDurationInSeconds * schedulerRate
			iotago.WithCongestionControlOptions(500, 500, 500, 800000, 500000, 100000, 1000, 100),
			iotago.WithWorkScoreOptions(25, 1, 100, 50, 10, 10, 50, 1, 10, 250),
		),
	),
	snapshotcreator.WithAddGenesisRootBlock(true),
}

var Docker = []options.Option[snapshotcreator.Options]{
	snapshotcreator.WithFilePath("docker-network.snapshot"),
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
			Amount:               mock.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x293dc170d9a59474e6d81cfba7f7d924c09b25d7166bcfba606e53114d0a758b")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount),
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
			Amount:               mock.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x05c1de274451db8de8182d64c6ee0dca3ae0c9077e0b4330c976976171d79064")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount),
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
			Amount:               mock.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x1e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount),
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
			Amount:               mock.MinIssuerAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x997be92a22b1933f36e26fba5f721756f95811d6b4ae21564197c2bfa4f28270")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			Mana:                 iotago.Mana(mock.MinIssuerAccountAmount),
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
	),
	snapshotcreator.WithProtocolParameters(
		iotago.NewV3ProtocolParameters(
			iotago.WithNetworkOptions("docker", "rms"),
			iotago.WithSupplyOptions(4_600_000_000_000_000, 1, 1, 10, 100, 100, 100),
			iotago.WithTimeProviderOptions(0, time.Now().Unix(), 10, 13),
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
		snapshotcreator.AccountDetails{
			/*
				node-01-validator

				Ed25519 Public Key:   01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe
				Ed25519 Address:      rms1qqlhggrg2ml9p0q5c4593r2yd3jwgxn20d65ulyw6z9r7xmm78apq4y2mxh
				Account Address:      rms1pqlhggrg2ml9p0q5c4593r2yd3jwgxn20d65ulyw6z9r7xmm78apq2067mf
				Restricted Address:   rms1xqqr7apqdpt0u59uznzkskydg3kxfeq6dfah2nnu3mgg50cm00cl5yqpqgrpy62q, Capabilities: mana
			*/
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe"))),
			Amount:               mock.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x01fb6b9db5d96240aef00bc950d1c67a6494513f6d7cf784e57b4972b96ab2fe")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{
			/*
				node-02-validator

				Ed25519 Public Key:   83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6
				Ed25519 Address:      rms1qzjwamvhjuqtw3dkfwmmj2fgcetcdyt4uxrnjxel4caxfstzz903ypu8xvn
				Account Address:      rms1pzjwamvhjuqtw3dkfwmmj2fgcetcdyt4uxrnjxel4caxfstzz903y7hhr3d
				Restricted Address:   rms1xqq2fmhdj7tspd69ke9m0wff9rr90p53whscwwgm87hr5expvgg47yspqgm6whkx, Capabilities: mana
			*/
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6"))),
			Amount:               mock.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x83e7f71a440afd48981a8b4684ddae24434b7182ce5c47cfb56ac528525fd4b6")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{
			/*
				node-03-validator

				Ed25519 Public Key:   ac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805
				Ed25519 Address:      rms1qz6kedkxyw9md2cmp0wcdhvsxrn2e7gzuyly76ffymy4dhvtkm58qlkjupg
				Account Address:      rms1pz6kedkxyw9md2cmp0wcdhvsxrn2e7gzuyly76ffymy4dhvtkm58qqazeuk
				Restricted Address:   rms1xqqt2m9kcc3chd4trv9ampkajqcwdt8eqtsnunmf9ynvj4ka3wmwsuqpqgvp9zxl, Capabilities: mana
			*/
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0xac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0xac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805"))),
			Amount:               mock.MinValidatorAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0xac628986b2ef52a1679f2289fcd7b4198476976dea4c30ae34ff04ae52e14805")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount),
		},
		snapshotcreator.AccountDetails{
			/*
				inx-blockissuer

				Ed25519 Public Key:   670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25
				Ed25519 Address:      rms1qqas0clgfsf8du9e6dw0yx9nwclqe0dd4f728pvgmcshpscm8r5mkjxnx5x
				Account Address:      rms1pqas0clgfsf8du9e6dw0yx9nwclqe0dd4f728pvgmcshpscm8r5mkddrrfc
				Restricted Address:   rms1xqqrkplrapxpyahsh8f4eusckdmrur9a4k48egu93r0zzuxrrvuwnwcpqgj0nu8t, Capabilities: mana
			*/
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex("0x670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25"))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex("0x670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25"))),
			Amount:               mock.MinIssuerAccountAmount,
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex("0x670a1a20ddb02a6cec53ec3196bc7d5bd26df2f5a6ca90b5fffd71364f104b25")))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			Mana:                 iotago.Mana(mock.MinIssuerAccountAmount),
		},
	),
	snapshotcreator.WithBasicOutputs(
		/*
			inx-faucet

			Ed25519 Public Key:   dcd760a51cfafe901f4ca0745d399af7146028af643e8a339c7bb82fbb1be7f9
			Ed25519 Address:      rms1qpy2e4my7cn9ydjx6hx0ytuq06tdxzm6kqry7dctvmagzxv9np0vg9c55n4
			Restricted Address:   rms1xqqy3txhvnmzv53kgm2ueu30splfd5ct02cqvnehpdn04qgeskv9a3qpqgrhlhv3, Capabilities: mana
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
			iotago.WithTimeProviderOptions(666666, time.Now().Unix(), 10, 13),
			iotago.WithLivenessOptions(30, 30, 10, 20, 30),
			// increase/decrease threshold = fraction * slotDurationInSeconds * schedulerRate
			iotago.WithCongestionControlOptions(500, 500, 500, 800000, 500000, 100000, 1000, 100),
			iotago.WithWorkScoreOptions(25, 1, 100, 50, 10, 10, 50, 1, 10, 250),
		),
	),
}
