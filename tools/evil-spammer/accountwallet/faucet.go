package accountwallet

import (
	"crypto/ed25519"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func (a *AccountWallet) RequestFaucetFunds(clt models.Client, receiveAddr iotago.Address, amount iotago.BaseToken) (*models.Output, error) {
	signedTx, err := a.faucet.prepareFaucetRequest(receiveAddr, amount)
	if err != nil {
		log.Errorf("failed to prepare faucet request: %s", err)

		return nil, err
	}

	issuerResp, err := a.client.GetBlockIssuance()
	if err != nil {
		log.Errorf("failed to get block issuance: %s", err)

		return nil, err
	}
	congestionResp, err := a.client.GetCongestion(a.faucet.account.ID())
	if err != nil {
		log.Errorf("failed to get congestion: %s", err)

		return nil, err
	}

	signedBlock, err := a.createBlock(issuerResp, congestionResp, signedTx)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return nil, err
	}

	_, err = clt.PostBlock(signedBlock)
	if err != nil {
		log.Errorf("failed to post block: %s", err)

		return nil, err
	}

	// set remainder output to be reused by the Faucet wallet
	a.faucet.unspentOutput = &models.Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1),
		Address:      a.faucet.address,
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

func (a *AccountWallet) createBlock(issuerResp *apimodels.IssuanceBlockHeaderResponse, congestionResp *apimodels.CongestionResponse, payload iotago.Payload) (*iotago.ProtocolBlock, error) {
	commitmentID, err := issuerResp.Commitment.ID()
	if err != nil {
		return nil, fmt.Errorf("failed to get commitment ID: %w", err)
	}

	blockBuilder := builder.NewBasicBlockBuilder(a.api)
	blockBuilder.SlotCommitmentID(commitmentID)
	blockBuilder.LatestFinalizedSlot(issuerResp.LatestFinalizedSlot)
	blockBuilder.IssuingTime(time.Now())
	blockBuilder.StrongParents(issuerResp.StrongParents)
	fmt.Printf("WeakParents len: %v\n", len(issuerResp.WeakParents))
	blockBuilder.WeakParents(issuerResp.WeakParents)
	fmt.Printf("ShallowLikeParents len: %v\n", len(issuerResp.ShallowLikeParents))
	blockBuilder.ShallowLikeParents(issuerResp.ShallowLikeParents)
	blockBuilder.Payload(payload)
	blockBuilder.MaxBurnedMana(congestionResp.ReferenceManaCost)
	blockBuilder.Sign(a.faucet.account.ID(), a.faucet.account.PrivateKey())

	blk, err := blockBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build block: %w", err)
	}

	return blk, nil
}

type faucet struct {
	address       *iotago.Ed25519Address
	unspentOutput *models.Output
	account       blockfactory.Account

	seed []byte
	clt  models.Client

	sync.Mutex
}

func newFaucet(clt models.Client, hexFaucetUnspentOutputID string) *faucet {
	//get Faucet output and amount
	var faucetAmount iotago.BaseToken

	faucetUnspentOutputID, err := iotago.OutputIDFromHex(hexFaucetUnspentOutputID)
	if err != nil {
		log.Warnf("Cannot parse faucet output id from config: %v", err)
	}

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

	f := &faucet{
		seed: dockerGenesisSeed(),
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

	f.createFaucetAccountFromSeed()

	return f
}

func (f *faucet) createFaucetAccountFromSeed() {
	privateKey := ed25519.NewKeyFromSeed(f.seed[:])
	ed25519PubKey := privateKey.Public().(ed25519.PublicKey)
	accIdBytes := blake2b.Sum256(ed25519PubKey[:])

	var accountID iotago.AccountID
	copy(accountID[:], accIdBytes[:iotago.AccountIDLength])

	f.account = blockfactory.NewEd25519Account(accountID, privateKey)
}

func (f *faucet) prepareFaucetRequest(receiveAddr iotago.Address, amount iotago.BaseToken) (*iotago.SignedTransaction, error) {
	remainderAmount := f.unspentOutput.Balance - amount

	signedTx, err := f.createFaucetTransaction(receiveAddr, amount, remainderAmount)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func (f *faucet) createFaucetTransaction(receiveAddr iotago.Address, amount iotago.BaseToken, remainderAmount iotago.BaseToken) (*iotago.SignedTransaction, error) {
	txBuilder := builder.NewTransactionBuilder(f.clt.CurrentAPI())

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
	txBuilder.SetCreationSlot(f.clt.CurrentAPI().TimeProvider().SlotFromTime(time.Now()))

	hdWallet := mock.NewHDWallet("", f.seed[:], 0)
	signedTx, err := txBuilder.Build(hdWallet.AddressSigner())
	if err != nil {
		log.Errorf("failed to build transaction: %s", err)

		return nil, err
	}
	return signedTx, err
}
