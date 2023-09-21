package dashboard

import (
	"encoding/json"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Address //////////////////////////////////////////////////////////////////////////////////////////////////////

// Address represents the JSON model of a ledgerstate.Address.
// type Address struct {
// 	Type   string `json:"type"`
// 	Base58 string `json:"base58"`
// }

// // NewAddress returns an Address from the given ledgerstate.Address.
// func NewAddress(address devnetvm.Address) *Address {
// 	return &Address{
// 		Type:   address.Type().String(),
// 		Base58: address.Base58(),
// 	}
// }

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Output ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Output represents the JSON model of a ledgerstate.Output.
type Output struct {
	OutputID *OutputID         `json:"outputID,omitempty"`
	Type     iotago.OutputType `json:"type"`
	Output   json.RawMessage   `json:"output"`
}

// NewOutput returns an Output from the given ledgerstate.Output.
func NewOutput(output iotago.Output) (result *Output) {
	outputJSON, err := deps.Protocol.CurrentAPI().JSONEncode(output)
	if err != nil {
		return nil
	}

	return &Output{
		Type:   output.Type(),
		Output: outputJSON,
	}
}

// NewOutput returns an Output from the given ledgerstate.Output.
func NewOutputFromLedgerstateOutput(output *utxoledger.Output) (result *Output) {
	outputJSON, err := deps.Protocol.APIForSlot(output.SlotCreated()).JSONEncode(output)
	if err != nil {
		return nil
	}

	return &Output{
		OutputID: NewOutputID(output.OutputID()),
		Type:     output.OutputType(),
		Output:   outputJSON,
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region OutputID /////////////////////////////////////////////////////////////////////////////////////////////////////

// OutputID represents the JSON model of a ledgerstate.OutputID.
type OutputID struct {
	Hex           string `json:"hex"`
	TransactionID string `json:"transactionID"`
	OutputIndex   uint16 `json:"outputIndex"`
}

// NewOutputID returns an OutputID from the given ledgerstate.OutputID.
func NewOutputID(outputID iotago.OutputID) *OutputID {
	return &OutputID{
		Hex:           outputID.ToHex(),
		TransactionID: outputID.TransactionID().ToHex(),
		OutputIndex:   outputID.Index(),
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region OutputMetadata ///////////////////////////////////////////////////////////////////////////////////////////////

// OutputMetadata represents the JSON model of the mempool.OutputMetadata.
// type OutputMetadata struct {
// 	OutputID              *OutputID          `json:"outputID"`
// 	ConflictIDs           []string           `json:"conflictIDs"`
// 	FirstConsumer         string             `json:"firstCount"`
// 	ConfirmedConsumer     string             `json:"confirmedConsumer,omitempty"`
// 	ConfirmationState     confirmation.State `json:"confirmationState"`
// 	ConfirmationStateTime int64              `json:"confirmationStateTime"`
// }

// // NewOutputMetadata returns the OutputMetadata from the given mempool.OutputMetadata.
// func NewOutputMetadata(outputMetadata *mempool.OutputMetadata, confirmedConsumerID utxo.TransactionID) *OutputMetadata {
// 	return &OutputMetadata{
// 		OutputID: NewOutputID(outputMetadata.ID()),
// 		ConflictIDs: lo.Map(lo.Map(outputMetadata.ConflictIDs().Slice(), func(t utxo.TransactionID) []byte {
// 			return lo.PanicOnErr(t.Bytes())
// 		}), base58.Encode),
// 		FirstConsumer:         outputMetadata.FirstConsumer().Base58(),
// 		ConfirmedConsumer:     confirmedConsumerID.Base58(),
// 		ConfirmationState:     outputMetadata.ConfirmationState(),
// 		ConfirmationStateTime: outputMetadata.ConfirmationStateTime().Unix(),
// 	}
// }

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Transaction //////////////////////////////////////////////////////////////////////////////////////////////////

// Transaction represents the JSON model of a ledgerstate.Transaction.
type Transaction struct {
	TransactionID    string                  `json:"txId"`
	NetworkID        iotago.NetworkID        `json:"networkId"`
	CreationSlot     iotago.SlotIndex        `json:"creationSlot"`
	Inputs           []*Input                `json:"inputs"`
	InputsCommitment iotago.InputsCommitment `json:"inputsCommitment"`
	Outputs          []*Output               `json:"outputs"`
	Unlocks          []*UnlockBlock          `json:"unlocks"`
	Payload          []byte                  `json:"payload"`
}

// NewTransaction returns a Transaction from the given ledgerstate.Transaction.
func NewTransaction(iotaTx *iotago.Transaction) *Transaction {
	txID, err := iotaTx.ID(deps.Protocol.CurrentAPI())
	if err != nil {
		return nil
	}

	inputs := make([]*Input, len(iotaTx.Essence.Inputs))
	for i, input := range iotaTx.Essence.Inputs {
		inputs[i] = NewInput(input)
	}

	outputs := make([]*Output, len(iotaTx.Essence.Outputs))
	for i, output := range iotaTx.Essence.Outputs {
		outputs[i] = NewOutput(output)
		outputs[i].OutputID = &OutputID{
			Hex:           iotago.OutputIDFromTransactionIDAndIndex(txID, uint16(i)).ToHex(),
			TransactionID: txID.ToHex(),
			OutputIndex:   uint16(i),
		}
	}

	unlockBlocks := make([]*UnlockBlock, len(iotaTx.Unlocks))
	for i, unlockBlock := range iotaTx.Unlocks {
		unlockBlocks[i] = NewUnlockBlock(unlockBlock)
	}

	dataPayload := make([]byte, 0)
	if iotaTx.Essence.Payload != nil {
		dataPayload, _ = deps.Protocol.CurrentAPI().Encode(iotaTx.Essence.Payload)
	}

	return &Transaction{
		NetworkID:    iotaTx.Essence.NetworkID,
		CreationSlot: iotaTx.Essence.CreationSlot,
		Inputs:       inputs,
		Outputs:      outputs,
		Unlocks:      unlockBlocks,
		Payload:      dataPayload,
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Input ////////////////////////////////////////////////////////////////////////////////////////////////////////

// Input represents the JSON model of a ledgerstate.Input.
type Input struct {
	Type               string    `json:"type"`
	ReferencedOutputID *OutputID `json:"referencedOutputID,omitempty"`
	// the referenced output object, omit if not specified
	Output *Output `json:"output,omitempty"`
}

// NewInput returns an Input from the given ledgerstate.Input.
func NewInput(input iotago.Input) *Input {
	utxoInput, isUtxoInput := input.(*iotago.UTXOInput)
	if !isUtxoInput {
		return nil
	}

	return &Input{
		Type:               input.Type().String(),
		ReferencedOutputID: NewOutputID(utxoInput.OutputID()),
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region UnlockBlock //////////////////////////////////////////////////////////////////////////////////////////////////

// UnlockBlock represents the JSON model of a ledgerstate.UnlockBlock.
type UnlockBlock struct {
	Type          string               `json:"type"`
	SignatureType iotago.SignatureType `json:"signatureType,omitempty"`
	Signature     json.RawMessage      `json:"signature,omitempty"`
}

// NewUnlockBlock returns an UnlockBlock from the given ledgerstate.UnlockBlock.
func NewUnlockBlock(unlockBlock iotago.Unlock) *UnlockBlock {
	result := &UnlockBlock{
		Type: unlockBlock.Type().String(),
	}

	switch unlock := unlockBlock.(type) {
	case *iotago.SignatureUnlock:
		switch signature := unlock.Signature.(type) {
		case *iotago.Ed25519Signature:
			sigJSON, err := deps.Protocol.CurrentAPI().JSONEncode(signature)
			if err != nil {
				return nil
			}
			result.Signature = sigJSON
			result.SignatureType = iotago.SignatureEd25519
		}
	}

	return result
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TransactionMetadata ///////////////////////////////////////////////////////////////////////////////////////////

// TransactionMetadata represents the JSON model of the mempool.TransactionMetadata.
type TransactionMetadata struct {
	TransactionID         string   `json:"transactionID"`
	ConflictIDs           []string `json:"conflictIDs"`
	Booked                bool     `json:"booked"`
	BookedTime            int64    `json:"bookedTime"`
	ConfirmationState     string   `json:"confirmationState"`
	ConfirmationStateTime int64    `json:"confirmationStateTime"`
}

// NewTransactionMetadata returns the TransactionMetadata from the given mempool.TransactionMetadata.
func NewTransactionMetadata(transactionMetadata mempool.TransactionMetadata, conflicts ds.Set[iotago.Identifier]) *TransactionMetadata {
	var confirmationState string
	if transactionMetadata.IsAccepted() {
		confirmationState = "accepted"
	} else if transactionMetadata.IsPending() {
		confirmationState = "pending"
	} else if transactionMetadata.IsRejected() {
		confirmationState = "rejected"
	}

	return &TransactionMetadata{
		TransactionID: transactionMetadata.ID().ToHex(),
		ConflictIDs: func() []string {
			var strIDs []string
			for _, txID := range conflicts.ToSlice() {
				strIDs = append(strIDs, txID.ToHex())
			}
			return strIDs
		}(),
		Booked:            transactionMetadata.IsBooked(),
		ConfirmationState: confirmationState,
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

type SlotDetailsResponse struct {
	Index            uint64   `json:"index"`
	PrevID           string   `json:"prevID"`
	RootsID          string   `json:"rootsID"`
	CumulativeWeight uint64   `json:"cumulativeWeight"`
	CreatedOutputs   []string `json:"createdOutputs"`
	SpentOutputs     []string `json:"spentOutputs"`
}

func NewSlotDetails(commitment *model.Commitment, diffs *utxoledger.SlotDiff) *SlotDetailsResponse {
	createdOutputs := make([]string, len(diffs.Outputs))
	consumedOutputs := make([]string, len(diffs.Spents))

	for i, output := range diffs.Outputs {
		createdOutputs[i] = output.OutputID().ToHex()
	}

	for i, output := range diffs.Spents {
		consumedOutputs[i] = output.OutputID().ToHex()
	}

	return &SlotDetailsResponse{
		Index:            uint64(commitment.Index()),
		PrevID:           commitment.PreviousCommitmentID().ToHex(),
		RootsID:          commitment.RootsID().ToHex(),
		CumulativeWeight: commitment.CumulativeWeight(),
		CreatedOutputs:   createdOutputs,
		SpentOutputs:     consumedOutputs,
	}
}
