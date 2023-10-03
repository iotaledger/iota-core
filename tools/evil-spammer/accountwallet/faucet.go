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
	address       *iotago.Ed25519Address
	unspentOutput *models.Output
	account       iotago.AccountID

	seed []byte
	clt  models.Client

	sync.Mutex
}

func NewFaucet(clt models.Client, faucetUnspentOutputID iotago.OutputID) *Faucet {
	//get Faucet output and amount
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
	f.address = hdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)
	f.unspentOutput = &models.Output{
		Address:      f.address,
		Index:        0,
		OutputID:     faucetUnspentOutputID,
		Balance:      faucetAmount,
		OutputStruct: faucetOutput,
	}

	return f
}

func (f *Faucet) RequestFunds(clt models.Client, receiveAddr iotago.Address, amount iotago.BaseToken) (*models.Output, error) {
	remainderAmount := f.unspentOutput.Balance - amount

	txBuilder := builder.NewTransactionBuilder(clt.CurrentAPI())

	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: f.address,
		InputID:      f.unspentOutput.OutputID,
		Input:        f.unspentOutput.OutputStruct,
	})

	switch receiveAddr.(type) {
	case *iotago.Ed25519Address:
		txBuilder.AddOutput(&iotago.BasicOutput{
			Amount: amount,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: receiveAddr},
			},
		})
	case *iotago.ImplicitAccountCreationAddress:
		log.Infof("creating account %s", receiveAddr)
		accOutputBuilder := builder.NewAccountOutputBuilder(receiveAddr, receiveAddr, amount)
		output, err := accOutputBuilder.Build()
		if err != nil {
			log.Errorf("failed to build account output: %s", err)

			return nil, err
		}
		txBuilder.AddOutput(output)
	}

	// remainder output
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: remainderAmount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: f.address},
		},
	})

	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("Faucet funds"), Data: []byte("to addr" + receiveAddr.String())})
	txBuilder.SetCreationSlot(clt.CurrentAPI().TimeProvider().SlotFromTime(time.Now()))

	hdWallet := mock.NewHDWallet("", f.seed[:], 0)
	signedTx, err := txBuilder.Build(hdWallet.AddressSigner())
	if err != nil {
		log.Errorf("failed to build transaction: %s", err)

		return nil, err
	}

	// send transaction
	_, err = clt.PostTransaction(signedTx)
	if err != nil {
		log.Errorf("failed to post transaction: %s", err)

		return nil, err
	}

	// set remainder output to be reused by the Faucet wallet
	f.unspentOutput = &models.Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1),
		Address:      f.address,
		Index:        0,
		Balance:      signedTx.Transaction.Outputs[1].BaseTokenAmount(),
		OutputStruct: signedTx.Transaction.Outputs[1],
	}

	return &models.Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0),
		Address:      receiveAddr,
		Index:        0,
		Balance:      signedTx.Transaction.Outputs[0].BaseTokenAmount(),
		OutputStruct: signedTx.Transaction.Outputs[0],
	}, nil
}
