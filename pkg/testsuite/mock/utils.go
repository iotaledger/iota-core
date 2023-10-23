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
			case iotago.OutputAccount:
				// For alias we need to unlock the state controller
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

func WithAccountInput(input *utxoledger.Output, governorTransition bool) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		switch input.OutputType() {
		case iotago.OutputAccount:
			address := input.Output().UnlockConditionSet().StateControllerAddress().Address
			if governorTransition {
				address = input.Output().UnlockConditionSet().GovernorAddress().Address
			}
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
			txBuilder.IncreaseAllotment(allotment.AccountID, allotment.Value)
		}
	}
}

func WithSlotCreated(creationSlot iotago.SlotIndex) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.SetCreationSlot(creationSlot)
	}
}

func WithContextInputs(contextInputs iotago.TxEssenceContextInputs) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, input := range contextInputs {
			txBuilder.AddContextInput(input)
		}
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
