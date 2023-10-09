package accountwallet

import (
	"fmt"
	"sync"
	"time"

	"github.com/mr-tron/base58"

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

	issuerResp, congestionResp, err := a.requestBlockData()
	if err != nil {
		return nil, err
	}

	signedBlock, err := a.createBlock(issuerResp, congestionResp, signedTx, a.faucet.account)
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
		Address:      a.faucet.genesisHdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address),
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

func (a *AccountWallet) requestBlockData() (*apimodels.IssuanceBlockHeaderResponse, *apimodels.CongestionResponse, error) {
	issuerResp, err := a.client.GetBlockIssuance()
	if err != nil {
		log.Errorf("failed to get block issuance: %s", err)

		return nil, nil, err
	}

	congestionResp, err := a.client.GetCongestion(a.faucet.account.ID())
	if err != nil {
		log.Errorf("failed to get congestion: %s", err)

		return nil, nil, err
	}

	return issuerResp, congestionResp, nil
}

func (a *AccountWallet) PostBlock(clt models.Client, payload iotago.Payload, issuer blockfactory.Account) error {
	issuerResp, congestionResp, err := a.requestBlockData()
	if err != nil {
		return err
	}

	signedBlock, err := a.createBlock(issuerResp, congestionResp, payload, issuer)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return err
	}

	_, err = clt.PostBlock(signedBlock)
	if err != nil {
		log.Errorf("failed to post block: %s", err)

		return err
	}

	return nil

}

func (a *AccountWallet) createBlock(issuerResp *apimodels.IssuanceBlockHeaderResponse, congestionResp *apimodels.CongestionResponse, payload iotago.Payload, issuer blockfactory.Account) (*iotago.ProtocolBlock, error) {
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
	blockBuilder.Sign(issuer.ID(), issuer.PrivateKey())

	blk, err := blockBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build block: %w", err)
	}

	return blk, nil
}

type faucetParams struct {
	latestUsedOutputID string
	faucetPrivateKey   string
	faucetAccountID    string
	genesisSeed        string
}

type faucet struct {
	unspentOutput   *models.Output
	account         blockfactory.Account
	genesisHdWallet *mock.HDWallet

	clt models.Client

	sync.Mutex
}

func newFaucet(clt models.Client, faucetParams *faucetParams) *faucet {
	//get Faucet output and amount
	var faucetAmount iotago.BaseToken

	faucetUnspentOutputID, err := iotago.OutputIDFromHex(faucetParams.latestUsedOutputID)
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
	genesisSeed, err := base58.Decode(faucetParams.genesisSeed)
	if err != nil {
		fmt.Printf("failed to decode base58 seed, using the default one: %v", err)
	}

	f := &faucet{
		clt:             clt,
		account:         blockfactory.AccountFromParams(faucetParams.faucetAccountID, faucetParams.faucetPrivateKey),
		genesisHdWallet: mock.NewHDWallet("", genesisSeed, 0),
	}

	f.genesisHdWallet.Address()
	f.unspentOutput = &models.Output{
		Address:      f.genesisHdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address),
		Index:        0,
		OutputID:     faucetUnspentOutputID,
		Balance:      faucetAmount,
		OutputStruct: faucetOutput,
	}

	return f
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
		UnlockTarget: f.genesisHdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address),
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
			&iotago.AddressUnlockCondition{Address: f.genesisHdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)},
		},
	})

	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("Faucet funds"), Data: []byte("to addr" + receiveAddr.String())})
	txBuilder.SetCreationSlot(f.clt.CurrentAPI().TimeProvider().SlotFromTime(time.Now()))

	signedTx, err := txBuilder.Build(f.genesisHdWallet.AddressSigner())
	if err != nil {
		log.Errorf("failed to build transaction: %s", err)

		return nil, err
	}
	return signedTx, err
}
