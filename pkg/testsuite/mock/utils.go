package mock

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

const MinIssuerAccountAmount = iotago.BaseToken(372900)
const MinValidatorAccountAmount = iotago.BaseToken(722800)
const AccountConversionManaCost = iotago.Mana(1000000)
const MaxBlockManaCost = iotago.Mana(1000000)

// TransactionBuilder options

func WithInputs(inputs utxoledger.Outputs) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, input := range inputs {
			switch input.OutputType() {
			case iotago.OutputFoundry:
				// For foundries we need to unlock the account output
				txBuilder.AddInput(&builder.TxInput{
					UnlockTarget: input.Output().UnlockConditionSet().ImmutableAccount().Address,
					InputID:      input.OutputID(),
					Input:        input.Output(),
				})
			case iotago.OutputAnchor:
				// For anchor outputs we need to unlock the state controller
				txBuilder.AddInput(&builder.TxInput{
					UnlockTarget: input.Output().UnlockConditionSet().StateControllerAddress().Address,
					InputID:      input.OutputID(),
					Input:        input.Output(),
				})
			default:
				txBuilder.AddInput(&builder.TxInput{
					UnlockTarget: input.Output().UnlockConditionSet().Address().Address,
					InputID:      input.OutputID(),
					Input:        input.Output(),
				})
			}
		}
	}
}

func WithAccountInput(input *utxoledger.Output) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		switch input.OutputType() {
		case iotago.OutputAccount:
			address := input.Output().UnlockConditionSet().Address().Address

			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: address,
				InputID:      input.OutputID(),
				Input:        input.Output(),
			})
		default:
			panic("only OutputAccount can be added as account input")
		}
	}
}

func WithAllotments(allotments iotago.Allotments) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, allotment := range allotments {
			txBuilder.IncreaseAllotment(allotment.AccountID, allotment.Mana)
		}
	}
}

func WithCreationSlot(creationSlot iotago.SlotIndex) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.SetCreationSlot(creationSlot)
	}
}

func WithCommitmentInput(input *iotago.CommitmentInput) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.AddCommitmentInput(input)
	}
}

func WithBlockIssuanceCreditInput(input *iotago.BlockIssuanceCreditInput) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.AddBlockIssuanceCreditInput(input)
	}
}

func WithRewardInput(input *iotago.RewardInput, mana iotago.Mana) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.AddRewardInput(input, mana)
	}
}

func WithOutputs(outputs iotago.Outputs[iotago.Output]) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, output := range outputs {
			txBuilder.AddOutput(output)
		}
	}
}

func WithTaggedDataPayload(payload *iotago.TaggedData) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.AddTaggedDataPayload(payload)
	}
}

func WithAllotAllManaToAccount(slot iotago.SlotIndex, accountID iotago.AccountID) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.AllotAllMana(slot, accountID)
	}
}

// DelegationOutput options

func WithDelegatedAmount(delegatedAmount iotago.BaseToken) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.DelegatedAmount(delegatedAmount)
	}
}

func WithDelegatedValidatorAddress(validatorAddress *iotago.AccountAddress) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.ValidatorAddress(validatorAddress)
	}
}

func WithDelegationStartEpoch(startEpoch iotago.EpochIndex) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.StartEpoch(startEpoch)
	}
}

func WithDelegationEndEpoch(endEpoch iotago.EpochIndex) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.EndEpoch(endEpoch)
	}
}

func WithDelegationConditions(delegationConditions iotago.DelegationOutputUnlockConditions) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.Address(delegationConditions.MustSet().Address().Address)
	}
}

func WithDelegationAmount(amount iotago.BaseToken) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.Amount(amount)
	}
}

// BlockIssuer options

func WithBlockIssuerFeature(keys iotago.BlockIssuerKeys, expirySlot iotago.SlotIndex) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.BlockIssuer(keys, expirySlot)
	}
}

func WithAddBlockIssuerKey(key iotago.BlockIssuerKey) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.BlockIssuerTransition().AddKeys(key)
	}
}

func WithBlockIssuerKeys(keys iotago.BlockIssuerKeys) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.BlockIssuerTransition().Keys(keys)
	}
}

func WithBlockIssuerExpirySlot(expirySlot iotago.SlotIndex) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.BlockIssuerTransition().ExpirySlot(expirySlot)
	}
}

func WithoutBlockIssuerFeature() options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.RemoveFeature(iotago.FeatureBlockIssuer)
	}
}

func WithStakingFeature(amount iotago.BaseToken, fixedCost iotago.Mana, startEpoch iotago.EpochIndex, optEndEpoch ...iotago.EpochIndex) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.Staking(amount, fixedCost, startEpoch, optEndEpoch...)
	}
}

func WithoutStakingFeature() options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.RemoveFeature(iotago.FeatureStaking)
	}
}

// Account options

func WithAccountMana(mana iotago.Mana) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.Mana(mana)
	}
}

func WithAccountAmount(amount iotago.BaseToken) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.Amount(amount)
	}
}

func WithAccountIncreasedFoundryCounter(diff uint32) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.FoundriesToGenerate(diff)
	}
}

func WithAccountImmutableFeatures(features iotago.AccountOutputImmFeatures) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		for _, feature := range features.MustSet() {
			switch feature.Type() {
			case iotago.FeatureMetadata:
				//nolint:forcetypeassert
				accountBuilder.ImmutableMetadata(feature.(*iotago.MetadataFeature).Data)
			case iotago.FeatureSender:
				//nolint:forcetypeassert
				accountBuilder.ImmutableSender(feature.(*iotago.SenderFeature).Address)
			}
		}
	}
}

func WithAccountConditions(conditions iotago.AccountOutputUnlockConditions) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		for _, condition := range conditions.MustSet() {
			switch condition.Type() {
			case iotago.UnlockConditionAddress:
				//nolint:forcetypeassert
				accountBuilder.Address(condition.(*iotago.AddressUnlockCondition).Address)
			}
		}
	}
}
