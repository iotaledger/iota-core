package presets

import (
	"fmt"
	"os"

	"golang.org/x/crypto/blake2b"
	"gopkg.in/yaml.v3"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
)

type ValidatorYaml struct {
	Name      string `yaml:"name"`
	PublicKey string `yaml:"public-key"`
}

type BlockIssuerYaml struct {
	Name      string `yaml:"name"`
	PublicKey string `yaml:"public-key"`
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
	BlockIssuers []BlockIssuerYaml `yaml:"block-issuers"`
	BasicOutputs []BasicOutputYaml `yaml:"basic-outputs"`
}

func GenerateFromYaml(hostsFile string) ([]options.Option[snapshotcreator.Options], error) {
	yamlFile, err := os.ReadFile(hostsFile)
	if err != nil {
		return nil, err
	}
	var configYaml ConfigYaml
	err = yaml.Unmarshal(yamlFile, &configYaml)
	if err != nil {
		return nil, err
	}

	var accounts []snapshotcreator.AccountDetails
	for _, validator := range configYaml.Validators {
		pubkey := validator.PublicKey
		fmt.Printf("adding validator %s with publicKey %s\n", validator.Name, pubkey)
		account := snapshotcreator.AccountDetails{
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Amount:               mock.MinValidatorAccountAmount(protocolParamsDocker),
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey)))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			StakingEndEpoch:      iotago.MaxEpochIndex,
			FixedCost:            1,
			StakedAmount:         mock.MinValidatorAccountAmount(protocolParamsDocker),
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(protocolParamsDocker)),
		}
		accounts = append(accounts, account)
	}

	for _, blockIssuer := range configYaml.BlockIssuers {
		pubkey := blockIssuer.PublicKey
		fmt.Printf("adding blockissueer %s with publicKey %s\n", blockIssuer.Name, pubkey)
		account := snapshotcreator.AccountDetails{
			AccountID:            blake2b.Sum256(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Address:              iotago.Ed25519AddressFromPubKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey))),
			Amount:               mock.MinValidatorAccountAmount(protocolParamsDocker),
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(lo.PanicOnErr(hexutil.DecodeHex(pubkey)))),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 4,
			Mana:                 iotago.Mana(mock.MinValidatorAccountAmount(protocolParamsDocker)),
		}
		accounts = append(accounts, account)
	}

	var basicOutputs []snapshotcreator.BasicOutputDetails
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
		snapshotcreator.WithProtocolParameters(protocolParamsDocker),
		snapshotcreator.WithAccounts(accounts...),
		snapshotcreator.WithBasicOutputs(basicOutputs...),
	}, nil
}
