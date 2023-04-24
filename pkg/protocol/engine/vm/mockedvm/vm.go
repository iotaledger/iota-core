package mockedvm

import (
	"iota-core/pkg/iotago"
	"iota-core/pkg/protocol/engine/vm"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
)

// ExecuteTransaction executes the Transaction and determines the Outputs from the given Inputs. It returns an error
// if the execution fails.
func ExecuteTransaction(transaction vm.StateTransition, inputs []vm.State, _ ...uint64) (outputs []vm.State, err error) {
	typedTransaction, ok := transaction.(*MockedTransaction)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction type in MockedVM")
	}

	outputs = make([]vm.State, typedTransaction.M.OutputCount)
	for i := uint16(0); i < typedTransaction.M.OutputCount; i++ {
		id, idErr := transaction.ID()
		if idErr != nil {
			return nil, xerrors.Errorf("error while retrieving transaction ID: %w", idErr)
		}

		outputs[i] = NewMockedOutput(id, i)
	}

	return outputs, nil
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

func RegisterWithAPI(api iotago.API) {
	//MockedTransactionType = payload.NewType(payloadtype.MockedTransaction, "MockedTransactionType")

	if err := api.Underlying().RegisterTypeSettings(MockedTransaction{}, serix.TypeSettings{}.WithObjectType(uint32(new(MockedTransaction).PayloadType()))); err != nil {
		panic(errors.Wrap(err, "error registering Transaction type settings"))
	}

	if err := api.Underlying().RegisterInterfaceObjects((iotago.Payload)(nil), new(MockedTransaction)); err != nil {
		panic(errors.Wrap(err, "error registering Transaction as Payload interface"))
	}

	// TODO: is the object type ok?
	if err := api.Underlying().RegisterTypeSettings(MockedOutput{}, serix.TypeSettings{}.WithObjectType(uint8(1))); err != nil {
		panic(errors.Wrap(err, "error registering ExtendedLockedOutput type settings"))
	}

	if err := api.Underlying().RegisterInterfaceObjects((*vm.State)(nil), new(MockedOutput)); err != nil {
		panic(errors.Wrap(err, "error registering utxo.Output interface implementations"))
	}
}
