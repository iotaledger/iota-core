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
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	FaucetAccountAlias = "faucet"
)

func (a *AccountWallet) RequestBlockBuiltData(clt *nodeclient.Client, issuerID iotago.AccountID) (*apimodels.CongestionResponse, *apimodels.IssuanceBlockHeaderResponse, iotago.Version, error) {
	congestionResp, err := clt.Congestion(context.Background(), issuerID)
	if err != nil {
		return nil, nil, 0, ierrors.Wrapf(err, "failed to get congestion data for issuer %s", issuerID.ToHex())
	}

	issuerResp, err := clt.BlockIssuance(context.Background())
	if err != nil {
		return nil, nil, 0, ierrors.Wrap(err, "failed to get block issuance data")
	}

	version := clt.APIForSlot(congestionResp.Slot).Version()

	return congestionResp, issuerResp, version, nil
}

func (a *AccountWallet) RequestFaucetFunds(clt models.Client, receiveAddr iotago.Address, amount iotago.BaseToken) (*models.Output, error) {
	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(clt.Client(), a.faucet.account.ID())
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block built data for issuer %s", a.faucet.account.ID().ToHex())
	}

	signedTx, err := a.faucet.prepareFaucetRequest(receiveAddr, amount, congestionResp.ReferenceManaCost)
	if err != nil {
		log.Errorf("failed to prepare faucet request: %s", err)

		return nil, err
	}

	blkID, err := a.PostWithBlock(clt, signedTx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return nil, err
	}
	fmt.Println("block sent:", blkID.ToHex())

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

func (a *AccountWallet) PostWithBlock(clt models.Client, payload iotago.Payload, issuer blockhandler.Account, congestionResp *apimodels.CongestionResponse, issuerResp *apimodels.IssuanceBlockHeaderResponse, version iotago.Version) (iotago.BlockID, error) {
	signedBlock, err := a.CreateBlock(payload, issuer, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return iotago.EmptyBlockID, err
	}

	blockID, err := clt.PostBlock(signedBlock)
	if err != nil {
		log.Errorf("failed to post block: %s", err)

		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

func (a *AccountWallet) CreateBlock(payload iotago.Payload, issuer blockhandler.Account, congestionResp *apimodels.CongestionResponse, issuerResp *apimodels.IssuanceBlockHeaderResponse, version iotago.Version) (*iotago.ProtocolBlock, error) {
	issuingTime := time.Now()
	issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := a.client.APIForSlot(issuingSlot)

	blockBuilder := builder.NewBasicBlockBuilder(apiForSlot)

	commitmentID, err := issuerResp.Commitment.ID()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get commitment id")
	}

	blockBuilder.ProtocolVersion(version)
	blockBuilder.SlotCommitmentID(commitmentID)
	blockBuilder.LatestFinalizedSlot(issuerResp.LatestFinalizedSlot)
	blockBuilder.IssuingTime(time.Now())
	blockBuilder.StrongParents(issuerResp.StrongParents)
	blockBuilder.WeakParents(issuerResp.WeakParents)
	blockBuilder.ShallowLikeParents(issuerResp.ShallowLikeParents)

	blockBuilder.Payload(payload)
	blockBuilder.CalculateAndSetMaxBurnedMana(congestionResp.ReferenceManaCost)
	blockBuilder.Sign(issuer.ID(), issuer.PrivateKey())

	blk, err := blockBuilder.Build()
	if err != nil {
		return nil, ierrors.Errorf("failed to build block: %w", err)
	}

	return blk, nil
}

type faucetParams struct {
	faucetPrivateKey string
	faucetAccountID  string
	genesisSeed      string
}

type faucet struct {
	unspentOutput   *models.Output
	account         blockhandler.Account
	genesisHdWallet *mock.HDWallet

	clt models.Client

	sync.Mutex
}

func newFaucet(clt models.Client, faucetParams *faucetParams) (*faucet, error) {
	genesisSeed, err := base58.Decode(faucetParams.genesisSeed)
	if err != nil {
		log.Warnf("failed to decode base58 seed, using the default one: %v", err)
	}
	faucetAddr := mock.NewHDWallet("", genesisSeed, 0).Address(iotago.AddressEd25519)

	f := &faucet{
		clt:             clt,
		account:         blockhandler.AccountFromParams(faucetParams.faucetAccountID, faucetParams.faucetPrivateKey),
		genesisHdWallet: mock.NewHDWallet("", genesisSeed, 0),
	}

	faucetUnspentOutput, faucetUnspentOutputID, faucetAmount, err := f.getGenesisOutputFromIndexer(clt, faucetAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get faucet output from indexer")
	}

	f.unspentOutput = &models.Output{
		Address:      faucetAddr.(*iotago.Ed25519Address),
		Index:        0,
		OutputID:     faucetUnspentOutputID,
		Balance:      faucetAmount,
		OutputStruct: faucetUnspentOutput,
	}

	return f, nil
}

func (f *faucet) getGenesisOutputFromIndexer(clt models.Client, faucetAddr iotago.DirectUnlockableAddress) (iotago.Output, iotago.OutputID, iotago.BaseToken, error) {
	indexer, err := clt.Indexer()
	if err != nil {
		panic(ierrors.Wrap(err, "failed to get indexer"))
	}

	results, err := indexer.Outputs(context.Background(), &apimodels.BasicOutputsQuery{
		AddressBech32: faucetAddr.Bech32(iotago.PrefixTestnet),
	})
	if err != nil {
		return nil, iotago.EmptyOutputID, 0, ierrors.Wrap(err, "failed to prepare faucet unspent outputs indexer request")
	}

	var (
		faucetUnspentOutput   iotago.Output
		faucetUnspentOutputID iotago.OutputID
		faucetAmount          iotago.BaseToken
	)
	for results.Next() {
		unspents, err := results.Outputs(context.TODO())
		if err != nil {
			return nil, iotago.EmptyOutputID, 0, ierrors.Wrap(err, "failed to get faucet unspent outputs")
		}

		faucetUnspentOutput = unspents[0]
		faucetAmount = faucetUnspentOutput.BaseTokenAmount()
		faucetUnspentOutputID = lo.Return1(results.Response.Items.OutputIDs())[0]
	}

	return faucetUnspentOutput, faucetUnspentOutputID, faucetAmount, nil
}

func (f *faucet) prepareFaucetRequest(receiveAddr iotago.Address, amount iotago.BaseToken, rmc iotago.Mana) (*iotago.SignedTransaction, error) {
	remainderAmount, err := safemath.SafeSub(f.unspentOutput.Balance, amount)
	if err != nil {
		panic(err)
	}

	txBuilder, remainderIndex, err := f.createFaucetTransactionNoManaHandling(receiveAddr, amount, remainderAmount)
	if err != nil {
		return nil, err
	}

	rmcAllotedTxBuilder := txBuilder.Clone()
	// faucet will allot exact mana to be burnt, rest of the mana is alloted to faucet output remainder
	rmcAllotedTxBuilder.AllotRequiredManaAndStoreRemainingManaInOutput(txBuilder.CreationSlot(), rmc, f.account.ID(), remainderIndex)

	var signedTx *iotago.SignedTransaction
	signedTx, err = rmcAllotedTxBuilder.Build(f.genesisHdWallet.AddressSigner())
	if err != nil {
		log.Infof("WARN: failed to build tx with min required mana allotted, genesis potential mana was not enough, fallback to faucet account")
		txBuilder.AllotAllMana(txBuilder.CreationSlot(), f.account.ID())
		if signedTx, err = txBuilder.Build(f.genesisHdWallet.AddressSigner()); err != nil {
			return nil, ierrors.Wrapf(err, "failed to build transaction with all mana allotted, after not having enough mana required based on RMC")
		}
	}

	return signedTx, nil
}

func (f *faucet) createFaucetTransactionNoManaHandling(receiveAddr iotago.Address, amount iotago.BaseToken, remainderAmount iotago.BaseToken) (*builder.TransactionBuilder, int, error) {
	currentTime := time.Now()
	currentSlot := f.clt.LatestAPI().TimeProvider().SlotFromTime(currentTime)

	apiForSlot := f.clt.APIForSlot(currentSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)

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

			return nil, 0, err
		}
		txBuilder.AddOutput(output)
	}

	// remainder output
	remainderIndex := 1
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: remainderAmount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: f.genesisHdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)},
		},
	})
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("Faucet funds"), Data: []byte("to addr" + receiveAddr.String())})
	txBuilder.SetCreationSlot(currentSlot)

	return txBuilder, remainderIndex, nil
}
