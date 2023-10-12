package accountwallet

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

const (
	FaucetAccountAlias = "faucet"
)

func (a *AccountWallet) RequestFaucetFunds(clt models.Client, receiveAddr iotago.Address, amount iotago.BaseToken) (*models.Output, error) {
	signedTx, err := a.faucet.prepareFaucetRequest(receiveAddr, amount)
	if err != nil {
		log.Errorf("failed to prepare faucet request: %s", err)

		return nil, err
	}

	_, err = a.PostWithBlock(clt, signedTx, a.faucet.account)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

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

func (a *AccountWallet) PostWithBlock(clt models.Client, payload iotago.Payload, issuer blockhandler.Account) (iotago.BlockID, error) {
	signedBlock, err := a.CreateBlock(clt.Client(), payload, issuer)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return iotago.EmptyBlockID, err
	}

	_, err = clt.PostBlock(signedBlock)
	if err != nil {
		log.Errorf("failed to post block: %s", err)

		return iotago.EmptyBlockID, err
	}

	blockID, err := signedBlock.ID()
	if err != nil {
		log.Errorf("failed to get block id: %s", err)

		return iotago.EmptyBlockID, err
	}

	return blockID, nil

}

// TODO: create validation blocks too
func (a *AccountWallet) CreateBlock(clt *nodeclient.Client, payload iotago.Payload, issuer blockhandler.Account) (*iotago.ProtocolBlock, error) {
	blockBuilder := builder.NewBasicBlockBuilder(a.api)

	congestionResp, err := clt.Congestion(context.Background(), issuer.ID())
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get congestion data for issuer %s", issuer.ID().ToHex())
	}
	// TODO: modify block issuance api to indicate the slot index for commitment, to make sure it maches with congestion response
	issuerResp, err := clt.BlockIssuance(context.Background())
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get block issuance data")
	}

	commitmentID, err := issuerResp.Commitment.ID()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get commitment id")
	}

	blockBuilder.ProtocolVersion(clt.CurrentAPI().ProtocolParameters().Version())
	blockBuilder.SlotCommitmentID(commitmentID)
	blockBuilder.LatestFinalizedSlot(issuerResp.LatestFinalizedSlot)
	blockBuilder.IssuingTime(time.Now())
	blockBuilder.StrongParents(issuerResp.StrongParents)
	blockBuilder.WeakParents(issuerResp.WeakParents)
	blockBuilder.ShallowLikeParents(issuerResp.ShallowLikeParents)
	blockBuilder.MaxBurnedMana(congestionResp.ReferenceManaCost)

	blockBuilder.Payload(payload)
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
	genesisOutputID    string
}

type faucet struct {
	unspentOutput   *models.Output
	account         blockhandler.Account
	genesisHdWallet *mock.HDWallet

	clt models.Client

	sync.Mutex
}

func newFaucet(clt models.Client, faucetParams *faucetParams) *faucet {
	//get Faucet output and amount
	var faucetAmount iotago.BaseToken

	faucetUnspentOutputID, err := iotago.OutputIDFromHexString(faucetParams.latestUsedOutputID)
	if err != nil {
		log.Warnf("Cannot parse faucet output id from config: %v", err)
	}

	faucetOutput := clt.GetOutput(faucetUnspentOutputID)
	if faucetOutput != nil {
		faucetAmount = faucetOutput.BaseTokenAmount()
	} else {
		// use the genesis output ID instead, if we relaunch the docker network
		faucetUnspentOutputID, err = iotago.OutputIDFromHexString(faucetParams.genesisOutputID)
		if err != nil {
			panic("cannot parse genesis output id, please update config file with genesis OutputID created in the snapshot")
		}
		faucetOutput = clt.GetOutput(faucetUnspentOutputID)
		if faucetOutput == nil {
			panic("cannot find faucet output")
		}
		faucetAmount = faucetOutput.BaseTokenAmount()
	}
	genesisSeed, err := base58.Decode(faucetParams.genesisSeed)
	if err != nil {
		fmt.Printf("failed to decode base58 seed, using the default one: %v", err)
	}

	f := &faucet{
		clt:             clt,
		account:         blockhandler.AccountFromParams(faucetParams.faucetAccountID, faucetParams.faucetPrivateKey),
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
	remainderAmount, err := safemath.SafeSub(f.unspentOutput.Balance, amount)
	if err != nil {
		panic(err)
	}

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
	// TODO: if we want to test allotting exactly the same amount as for burning mana we need to update and join tx and block creation brocess
	// txBuilder.AllotRequiredManaAndStoreRemainingManaInOutput()
	// BuildAndSwapToBlockBuilder
	signedTx, err := txBuilder.Build(f.genesisHdWallet.AddressSigner())
	if err != nil {
		log.Errorf("failed to build transaction: %s", err)

		return nil, err
	}
	return signedTx, err
}
