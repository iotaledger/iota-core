package accountwallet

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

type Faucet struct {
	facuetAddress *iotago.Ed25519Address
	seed          []byte
	faucerAddr    *iotago.Ed25519Address
	clt           models.Client

	unspentOutput *models.Output

	sync.Mutex
}

func NewFaucet(clt models.Client, faucetUnspentOutputID iotago.OutputID) *Faucet {
	//get faucet output and amount
	var faucetAmount iotago.BaseToken

	faucetOutput := clt.GetOutput(faucetUnspentOutputID)
	if faucetOutput != nil {
		faucetAmount = faucetOutput.BaseTokenAmount()
	} else {
		// use the genesis output ID instead, if we relaunch the docker network
		faucetUnspentOutputID = iotago.OutputIDFromTransactionIDAndIndex(genesisTransactionID, 0)
		faucetOutput = clt.GetOutput(faucetUnspentOutputID)
		if faucetOutput != nil {
			faucetAmount = faucetOutput.BaseTokenAmount()
		}
	}

	f := &Faucet{
		seed: dockerFaucetSeed(),
		clt:  clt,
	}

	hdWallet := mock.NewHDWallet("", f.seed[:], 0)
	f.facuetAddress = hdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)
	f.unspentOutput = &models.Output{
		Address:      f.faucerAddr,
		Index:        0,
		OutputID:     faucetUnspentOutputID,
		Balance:      faucetAmount,
		OutputStruct: faucetOutput,
	}

	return f
}

func (f *Faucet) RequestFunds(receiveAddr iotago.Address, amount iotago.BaseToken) (*models.Output, error) {
	remainderAmount := f.unspentOutput.Balance - amount

	txBuilder := builder.NewTransactionBuilder(f.clt.CurrentAPI())

	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: f.facuetAddress,
		InputID:      f.unspentOutput.OutputID,
		Input:        f.unspentOutput.OutputStruct,
	})

	// receiver output
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: amount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: receiveAddr},
		},
	})

	// remainder output
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: remainderAmount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: f.facuetAddress},
		},
	})

	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("faucet funds"), Data: []byte("to addr" + receiveAddr.String())})
	txBuilder.SetCreationSlot(f.clt.CurrentAPI().TimeProvider().SlotFromTime(time.Now()))

	hdWallet := mock.NewHDWallet("", f.seed[:], 0)

	signedTx, err := txBuilder.Build(hdWallet.AddressSigner())
	if err != nil {
		return nil, err
	}

	// send transaction
	_, err = f.clt.PostTransaction(signedTx)
	if err != nil {
		return nil, err
	}

	// set remainder output to be reused by the faucet wallet
	f.unspentOutput = &models.Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.ID()), 1),
		Address:      f.facuetAddress,
		Index:        0,
		Balance:      signedTx.Transaction.Outputs[1].BaseTokenAmount(),
		OutputStruct: signedTx.Transaction.Outputs[1],
	}

	// TODO handle implicit acc creation
	//switch receiveAddr.(type) {
	//case *iotago.Ed25519Address:
	//case *iotago.ImplicitAccountCreationAddress:
	//
	//}

	return &models.Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.ID()), 0),
		Address:      receiveAddr,
		Index:        0,
		Balance:      signedTx.Transaction.Outputs[0].BaseTokenAmount(),
		OutputStruct: signedTx.Transaction.Outputs[0],
	}, nil
}
