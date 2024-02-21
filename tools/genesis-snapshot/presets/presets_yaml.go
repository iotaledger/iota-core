package presets

import (
	"fmt"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
)

type ValidatorYaml struct {
	Name      string `yaml:"name"`
	PublicKey string `yaml:"publicKey"`
}

type BlockIssuerYaml struct {
	Name      string `yaml:"name"`
	PublicKey string `yaml:"publicKey"`
}

type BasicOutputYaml struct {
	Address string `yaml:"address"`
	Amount  uint64 `yaml:"amount"`
	Mana    uint64 `yaml:"mana"`
}

type ConfigYaml struct {
	Name     string `yaml:"name"`
	FilePath string `yaml:"filepath"`

	Validators   []ValidatorYaml   `yaml:"validators"`
	BlockIssuers []BlockIssuerYaml `yaml:"blockIssuers"`
	BasicOutputs []BasicOutputYaml `yaml:"basicOutputs"`
}

func GenerateFromYaml(hostsFile string) ([]options.Option[snapshotcreator.Options], error) {
	var configYaml ConfigYaml
	if err := ioutils.ReadYAMLFromFile(hostsFile, &configYaml); err != nil {
		return nil, err
	}

	accounts := make([]snapshotcreator.AccountDetails, 0, len(configYaml.Validators)+len(configYaml.BlockIssuers))
	for _, validator := range configYaml.Validators {
		pubkey := validator.PublicKey
		fmt.Printf("adding validator %s with publicKey %s\n", validator.Name, pubkey)
		account := snapshotcreator.AccountDetails{
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Amount:               mock.MinValidatorAccountAmount(ProtocolParamsDocker),
			IssuerKey:            iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey)))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEndEpoch:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount(ProtocolParamsDocker),
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(ProtocolParamsDocker)),
		}
		accounts = append(accounts, account)
	}

	for _, blockIssuer := range configYaml.BlockIssuers {
		pubkey := blockIssuer.PublicKey
		fmt.Printf("adding blockissueer %s with publicKey %s\n", blockIssuer.Name, pubkey)
		account := snapshotcreator.AccountDetails{
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Amount:               mock.MinValidatorAccountAmount(ProtocolParamsDocker),
			IssuerKey:            iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey)))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(ProtocolParamsDocker)),
		}
		accounts = append(accounts, account)
	}

	basicOutputs := make([]snapshotcreator.BasicOutputDetails, 0, len(configYaml.BasicOutputs))
	for _, basicOutput := range configYaml.BasicOutputs {
		address := lo.Return2(iotago.ParseBech32(basicOutput.Address))
		amount := basicOutput.Amount
		mana := basicOutput.Mana
		fmt.Printf("adding basicOutput for %s with amount %d and mana %d\n", address, amount, mana)
		basicOutputs = append(basicOutputs, snapshotcreator.BasicOutputDetails{
			Address: address,
			Amount:  iotago.BaseToken(amount),
			Mana:    iotago.Mana(mana),
		})
	}

	return []options.Option[snapshotcreator.Options]{
		snapshotcreator.WithFilePath(configYaml.FilePath),
		snapshotcreator.WithProtocolParameters(ProtocolParamsDocker),
		snapshotcreator.WithAccounts(accounts...),
		snapshotcreator.WithBasicOutputs(basicOutputs...),
	}, nil
}
