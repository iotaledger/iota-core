package depositcalculator_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/depositcalculator"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestCalculate(t *testing.T) {

	protocolParams := iotago.NewV3SnapshotProtocolParameters(iotago.WithVersion(3))
	storageScoreStructure := iotago.NewStorageScoreStructure(protocolParams.StorageScoreParameters())

	type test struct {
		name         string
		outputType   iotago.OutputType
		options      []options.Option[depositcalculator.Options]
		targetOutput iotago.Output
		targetErr    error
	}

	tests := []*test{
		{
			name:       "ok - basic output",
			outputType: iotago.OutputBasic,
			options: []options.Option[depositcalculator.Options]{
				depositcalculator.WithAddress(&iotago.Ed25519Address{}),
			},
			targetOutput: &iotago.BasicOutput{
				UnlockConditions: iotago.BasicOutputUnlockConditions{
					&iotago.AddressUnlockCondition{Address: &iotago.Ed25519Address{}},
				},
			},
			targetErr: nil,
		},
		{
			name:       "ok - account output",
			outputType: iotago.OutputAccount,
			options: []options.Option[depositcalculator.Options]{
				depositcalculator.WithAddress(&iotago.Ed25519Address{}),
			},
			targetOutput: &iotago.AccountOutput{
				UnlockConditions: iotago.AccountOutputUnlockConditions{
					&iotago.AddressUnlockCondition{Address: &iotago.Ed25519Address{}},
				},
			},
			targetErr: nil,
		},
		{
			name:       "ok - account output - 2 block issuer keys",
			outputType: iotago.OutputAccount,
			options: []options.Option[depositcalculator.Options]{
				depositcalculator.WithAddress(&iotago.Ed25519Address{}),
				depositcalculator.WithBlockIssuerKeys(2),
			},
			targetOutput: &iotago.AccountOutput{
				UnlockConditions: iotago.AccountOutputUnlockConditions{
					&iotago.AddressUnlockCondition{Address: &iotago.Ed25519Address{}},
				},
				Features: iotago.AccountOutputFeatures{
					&iotago.BlockIssuerFeature{
						BlockIssuerKeys: iotago.BlockIssuerKeys{
							&iotago.Ed25519PublicKeyHashBlockIssuerKey{},
							&iotago.Ed25519PublicKeyHashBlockIssuerKey{},
						},
						ExpirySlot: 0,
					},
				},
			},
			targetErr: nil,
		},
		{
			name:       "ok - anchor output",
			outputType: iotago.OutputAnchor,
			options: []options.Option[depositcalculator.Options]{
				depositcalculator.WithStateControllerAddress(&iotago.Ed25519Address{}),
				depositcalculator.WithGovernorAddress(&iotago.Ed25519Address{}),
			},
			targetOutput: &iotago.AnchorOutput{
				UnlockConditions: iotago.AnchorOutputUnlockConditions{
					&iotago.StateControllerAddressUnlockCondition{Address: &iotago.Ed25519Address{}},
					&iotago.GovernorAddressUnlockCondition{Address: &iotago.Ed25519Address{}},
				},
			},
			targetErr: nil,
		},
		{
			name:       "ok - foundry output",
			outputType: iotago.OutputFoundry,
			options:    []options.Option[depositcalculator.Options]{},
			targetOutput: &iotago.FoundryOutput{
				UnlockConditions: iotago.FoundryOutputUnlockConditions{
					&iotago.ImmutableAccountUnlockCondition{Address: &iotago.AccountAddress{}},
				},
				TokenScheme: &iotago.SimpleTokenScheme{
					MintedTokens:  big.NewInt(0),
					MeltedTokens:  big.NewInt(0),
					MaximumSupply: big.NewInt(0),
				},
			},
			targetErr: nil,
		},
		{
			name:       "ok - NFT output",
			outputType: iotago.OutputNFT,
			options: []options.Option[depositcalculator.Options]{
				depositcalculator.WithAddress(&iotago.Ed25519Address{}),
			},
			targetOutput: &iotago.NFTOutput{
				UnlockConditions: iotago.NFTOutputUnlockConditions{
					&iotago.AddressUnlockCondition{Address: &iotago.Ed25519Address{}},
				},
			},
			targetErr: nil,
		},
		{
			name:       "ok - Delegation output",
			outputType: iotago.OutputDelegation,
			options: []options.Option[depositcalculator.Options]{
				depositcalculator.WithAddress(&iotago.Ed25519Address{}),
			},
			targetOutput: &iotago.DelegationOutput{
				UnlockConditions: iotago.DelegationOutputUnlockConditions{
					&iotago.AddressUnlockCondition{Address: &iotago.Ed25519Address{}},
				},
			},
			targetErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := depositcalculator.MinDeposit(protocolParams, test.outputType, test.options...)
			if test.targetErr != nil {
				require.ErrorIs(t, err, test.targetErr)
				return
			}
			require.NoError(t, err)

			targetDeposit, err := storageScoreStructure.MinDeposit(test.targetOutput)
			require.NoError(t, err)

			require.Equal(t, targetDeposit, result)
		})
	}
}
