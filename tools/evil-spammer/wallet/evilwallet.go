package wallet

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

const (
	// FaucetRequestSplitNumber defines the number of outputs to split from a faucet request.
	FaucetRequestSplitNumber                  = 100
	faucetTokensPerRequest   iotago.BaseToken = 1_000_000

	waitForConfirmation   = 150 * time.Second
	waitForSolidification = 150 * time.Second

	awaitConfirmationSleep   = 3 * time.Second
	awaitSolidificationSleep = time.Millisecond * 500

	WaitForTxSolid = 150 * time.Second
)

var (
	defaultClientsURLs = []string{"http://localhost:8080", "http://localhost:8090"}

	genesisTransactionID = iotago.TransactionIDRepresentingData(0, []byte("genesis"))

	dockerFaucetSeed = func() []byte {
		genesisSeed, err := base58.Decode("7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih")
		if err != nil {
			log.Fatal(ierrors.Errorf("failed to decode base58 seed, using the default one: %w", err))
		}

		return genesisSeed
	}
)

// region EvilWallet ///////////////////////////////////////////////////////////////////////////////////////////////////////

// EvilWallet provides a user-friendly way to do complicated double spend scenarios.
type EvilWallet struct {
	// faucet is the wallet of faucet
	faucet        *Wallet
	wallets       *Wallets
	connector     Connector
	outputManager *OutputManager
	aliasManager  *AliasManager

	optFaucetSeed            []byte
	optFaucetUnspentOutputID iotago.OutputID
	optsClientURLs           []string
}

// NewEvilWallet creates an EvilWallet instance.
func NewEvilWallet(opts ...options.Option[EvilWallet]) *EvilWallet {
	return options.Apply(&EvilWallet{
		wallets:                  NewWallets(),
		aliasManager:             NewAliasManager(),
		optFaucetSeed:            dockerFaucetSeed(),
		optFaucetUnspentOutputID: iotago.OutputIDFromTransactionIDAndIndex(genesisTransactionID, 0),
		optsClientURLs:           defaultClientsURLs,
	}, opts, func(w *EvilWallet) {
		connector := NewWebClients(w.optsClientURLs)
		w.connector = connector

		clt := w.connector.GetClient()

		w.outputManager = NewOutputManager(connector, w.wallets)

		w.faucet = NewWallet()
		w.faucet.seed = [32]byte(w.optFaucetSeed)

		// get faucet output and amount
		var faucetAmount iotago.BaseToken

		faucetOutput := clt.GetOutput(w.optFaucetUnspentOutputID)
		if faucetOutput != nil {
			faucetAmount = faucetOutput.BaseTokenAmount()
		} else {
			// use the genesis output ID instead, if we relaunch the docker network
			w.optFaucetUnspentOutputID = iotago.OutputIDFromTransactionIDAndIndex(genesisTransactionID, 0)
			faucetOutput = clt.GetOutput(w.optFaucetUnspentOutputID)
			if faucetOutput != nil {
				faucetAmount = faucetOutput.BaseTokenAmount()
			}
		}

		w.faucet.AddUnspentOutput(&Output{
			Address:      w.faucet.AddressOnIndex(0),
			Index:        0,
			OutputID:     w.optFaucetUnspentOutputID,
			Balance:      faucetAmount,
			OutputStruct: faucetOutput,
		})
	})
}

func (e *EvilWallet) LastFaucetUnspentOutput() iotago.OutputID {
	faucetAddr := e.faucet.AddressOnIndex(0)
	unspentFaucet := e.faucet.UnspentOutput(faucetAddr.String())

	return unspentFaucet.OutputID
}

// NewWallet creates a new wallet of the given wallet type.
func (e *EvilWallet) NewWallet(wType ...WalletType) *Wallet {
	walletType := Other
	if len(wType) != 0 {
		walletType = wType[0]
	}

	return e.wallets.NewWallet(walletType)
}

// GetClients returns the given number of clients.
func (e *EvilWallet) GetClients(num int) []Client {
	return e.connector.GetClients(num)
}

// Connector give access to the EvilWallet connector.
func (e *EvilWallet) Connector() Connector {
	return e.connector
}

func (e *EvilWallet) UnspentOutputsLeft(walletType WalletType) int {
	return e.wallets.UnspentOutputsLeft(walletType)
}

func (e *EvilWallet) NumOfClient() int {
	clts := e.connector.Clients()
	return len(clts)
}

func (e *EvilWallet) AddClient(clientURL string) {
	e.connector.AddClient(clientURL)
}

func (e *EvilWallet) RemoveClient(clientURL string) {
	e.connector.RemoveClient(clientURL)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region EvilWallet Faucet Requests ///////////////////////////////////////////////////////////////////////////////////

// RequestFundsFromFaucet requests funds from the faucet, then track the confirmed status of unspent output,
// also register the alias name for the unspent output if provided.
func (e *EvilWallet) RequestFundsFromFaucet(options ...FaucetRequestOption) (initWallet *Wallet, err error) {
	initWallet = e.NewWallet(Fresh)
	buildOptions := NewFaucetRequestOptions(options...)

	output, err := e.requestFaucetFunds(initWallet)
	if err != nil {
		return
	}

	if buildOptions.outputAliasName != "" {
		e.aliasManager.AddInputAlias(output, buildOptions.outputAliasName)
	}

	return
}

// RequestFreshBigFaucetWallets creates n new wallets, each wallet is created from one faucet request and contains 1000 outputs.
func (e *EvilWallet) RequestFreshBigFaucetWallets(numberOfWallets int) {
	// channel to block the number of concurrent goroutines
	semaphore := make(chan bool, 1)
	wg := sync.WaitGroup{}

	for reqNum := 0; reqNum < numberOfWallets; reqNum++ {
		wg.Add(1)
		// block if full
		semaphore <- true
		go func() {
			defer wg.Done()
			defer func() {
				// release
				<-semaphore
			}()

			err := e.RequestFreshBigFaucetWallet()
			if err != nil {
				return
			}
		}()
	}
	wg.Wait()
}

// RequestFreshBigFaucetWallet creates a new wallet and fills the wallet with 1000 outputs created from funds
// requested from the Faucet.
func (e *EvilWallet) RequestFreshBigFaucetWallet() error {
	initWallet := NewWallet()
	receiveWallet := e.NewWallet(Fresh)

	txIDs := make(iotago.TransactionIDs, 0)
	// TODO: calculate the exact number of required funds so we don't run out after a while
	for i := 0; i < 300; i++ {
		txID, err := e.requestAndSplitFaucetFunds(initWallet, receiveWallet)
		if err != nil {
			return ierrors.Wrap(err, "failed to request big funds from faucet")
		}

		txIDs = append(txIDs, txID)
	}

	e.outputManager.AwaitTransactionsConfirmation(txIDs...)

	e.wallets.SetWalletReady(receiveWallet)

	return nil
}

// RequestFreshFaucetWallet creates a new wallet and fills the wallet with 100 outputs created from funds
// requested from the Faucet.
func (e *EvilWallet) RequestFreshFaucetWallet() error {
	initWallet := NewWallet()
	receiveWallet := e.NewWallet(Fresh)
	txID, err := e.requestAndSplitFaucetFunds(initWallet, receiveWallet)
	if err != nil {
		return ierrors.Wrap(err, "failed to request funds from faucet")
	}

	e.outputManager.AwaitTransactionsConfirmation(txID)

	e.wallets.SetWalletReady(receiveWallet)

	return err
}

func (e *EvilWallet) requestAndSplitFaucetFunds(initWallet, receiveWallet *Wallet) (txID iotago.TransactionID, err error) {
	splitOutput, err := e.requestFaucetFunds(initWallet)
	if err != nil {
		return iotago.TransactionID{}, err
	}
	// first split 1 to FaucetRequestSplitNumber outputs
	return e.splitOutputs(splitOutput, initWallet, receiveWallet)
}

func (e *EvilWallet) requestFaucetFunds(wallet *Wallet) (outputID *Output, err error) {
	receiveAddr := wallet.AddressOnIndex(0)
	clt := e.connector.GetClient()

	faucetAddr := e.faucet.AddressOnIndex(0)
	unspentFaucet := e.faucet.UnspentOutput(faucetAddr.String())
	if unspentFaucet.OutputStruct == nil {
		clt := e.connector.GetClient()
		faucetOutput := clt.GetOutput(e.optFaucetUnspentOutputID)
		if faucetOutput == nil {
			panic("no valid faucet unspent output")
		}
		unspentFaucet.OutputStruct = faucetOutput
	}
	remainderAmount := unspentFaucet.Balance - faucetTokensPerRequest

	txBuilder := builder.NewTransactionBuilder(clt.CurrentAPI())

	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: faucetAddr,
		InputID:      unspentFaucet.OutputID,
		Input:        unspentFaucet.OutputStruct,
	})

	// receiver output
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: faucetTokensPerRequest,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: receiveAddr},
		},
	})

	// remainder output
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: remainderAmount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: faucetAddr},
		},
	})

	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("faucet funds"), Data: []byte("to addr" + receiveAddr.String())})
	txBuilder.SetCreationSlot(clt.CurrentAPI().TimeProvider().SlotFromTime(time.Now()))

	signedTx, err := txBuilder.Build(e.faucet.AddressSigner(faucetAddr))
	if err != nil {
		return nil, err
	}

	// send transaction
	_, err = clt.PostTransaction(signedTx)
	if err != nil {
		return nil, err
	}

	// requested output to split and use in spammer
	output := e.outputManager.CreateOutputFromAddress(wallet, receiveAddr, faucetTokensPerRequest, iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0), signedTx.Transaction.Outputs[0])

	// set remainder output to be reused by the faucet wallet
	e.faucet.AddUnspentOutput(&Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1),
		Address:      faucetAddr,
		Index:        0,
		Balance:      signedTx.Transaction.Outputs[1].BaseTokenAmount(),
		OutputStruct: signedTx.Transaction.Outputs[1],
	})

	return output, nil
}

// splitOutputs splits faucet input to 100 outputs.
func (e *EvilWallet) splitOutputs(splitOutput *Output, inputWallet, outputWallet *Wallet) (iotago.TransactionID, error) {
	if inputWallet.IsEmpty() {
		return iotago.TransactionID{}, ierrors.New("inputWallet is empty")
	}

	input, outputs := e.handleInputOutputDuringSplitOutputs(splitOutput, FaucetRequestSplitNumber, outputWallet)

	signedTx, err := e.CreateTransaction(WithInputs(input), WithOutputs(outputs), WithIssuer(inputWallet), WithOutputWallet(outputWallet))
	if err != nil {
		return iotago.TransactionID{}, err
	}

	_, err = e.connector.GetClient().PostTransaction(signedTx)
	if err != nil {
		fmt.Println(err)
		return iotago.TransactionID{}, err
	}

	return lo.PanicOnErr(signedTx.Transaction.ID()), nil
}

func (e *EvilWallet) handleInputOutputDuringSplitOutputs(splitOutput *Output, splitNumber int, receiveWallet *Wallet) (input *Output, outputs []*OutputOption) {
	input = splitOutput

	balances := SplitBalanceEqually(splitNumber, input.Balance)
	for _, bal := range balances {
		outputs = append(outputs, &OutputOption{amount: bal, address: receiveWallet.Address()})
	}

	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region EvilWallet functionality ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// ClearAliases remove only provided aliases from AliasManager.
func (e *EvilWallet) ClearAliases(aliases ScenarioAlias) {
	e.aliasManager.ClearAliases(aliases)
}

// ClearAllAliases remove all registered alias names.
func (e *EvilWallet) ClearAllAliases() {
	e.aliasManager.ClearAllAliases()
}

func (e *EvilWallet) PrepareCustomConflicts(conflictsMaps []ConflictSlice) (conflictBatch [][]*iotago.SignedTransaction, err error) {
	for _, conflictMap := range conflictsMaps {
		var txs []*iotago.SignedTransaction
		for _, conflictOptions := range conflictMap {
			tx, err2 := e.CreateTransaction(conflictOptions...)
			if err2 != nil {
				return nil, err2
			}
			txs = append(txs, tx)
		}
		conflictBatch = append(conflictBatch, txs)
	}

	return
}

// SendCustomConflicts sends transactions with the given conflictsMaps.
func (e *EvilWallet) SendCustomConflicts(conflictsMaps []ConflictSlice) (err error) {
	conflictBatch, err := e.PrepareCustomConflicts(conflictsMaps)
	if err != nil {
		return err
	}
	for _, txs := range conflictBatch {
		clients := e.connector.GetClients(len(txs))
		if len(txs) > len(clients) {
			return ierrors.New("insufficient clients to send conflicts")
		}

		// send transactions in parallel
		wg := sync.WaitGroup{}
		for i, tx := range txs {
			wg.Add(1)
			go func(clt Client, signedTx *iotago.SignedTransaction) {
				defer wg.Done()
				_, _ = clt.PostTransaction(signedTx)
			}(clients[i], tx)
		}
		wg.Wait()

		// wait until transactions are solid
		time.Sleep(WaitForTxSolid)
	}

	return
}

// CreateTransaction creates a transaction based on provided options. If no input wallet is provided, the next non-empty faucet wallet is used.
// Inputs of the transaction are determined in three ways:
// 1 - inputs are provided directly without associated alias, 2- alias is provided, and input is already stored in an alias manager,
// 3 - alias is provided, and there are no inputs assigned in Alias manager, so aliases are assigned to next ready inputs from input wallet.
func (e *EvilWallet) CreateTransaction(options ...Option) (signedTx *iotago.SignedTransaction, err error) {
	buildOptions, err := NewOptions(options...)
	if err != nil {
		return nil, err
	}
	// wallet used only for outputs in the middle of the batch, that will never be reused outside custom conflict batch creation.
	tempWallet := e.NewWallet()

	err = e.updateInputWallet(buildOptions)
	if err != nil {
		return nil, err
	}

	inputs, err := e.prepareInputs(buildOptions)
	if err != nil {
		return nil, err
	}

	outputs, addrAliasMap, tempAddresses, err := e.prepareOutputs(buildOptions, tempWallet)
	if err != nil {
		return nil, err
	}

	alias, remainder, remainderAddr, hasRemainder := e.prepareRemainderOutput(buildOptions, outputs)
	if hasRemainder {
		outputs = append(outputs, remainder)
		if alias != "" && addrAliasMap != nil {
			addrAliasMap[remainderAddr.String()] = alias
		}
	}

	signedTx, err = e.makeTransaction(inputs, outputs, buildOptions.inputWallet)
	if err != nil {
		return nil, err
	}

	e.addOutputsToOutputManager(signedTx, buildOptions.outputWallet, tempWallet, tempAddresses)
	e.registerOutputAliases(signedTx, addrAliasMap)

	return
}

// addOutputsToOutputManager adds output to the OutputManager if.
func (e *EvilWallet) addOutputsToOutputManager(signedTx *iotago.SignedTransaction, outWallet, tmpWallet *Wallet, tempAddresses map[string]types.Empty) {
	for idx, o := range signedTx.Transaction.Outputs {
		addr := o.UnlockConditionSet().Address().Address
		out := &Output{
			OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), uint16(idx)),
			Address:      addr,
			Balance:      o.BaseTokenAmount(),
			OutputStruct: o,
		}

		if _, ok := tempAddresses[addr.String()]; ok {
			e.outputManager.AddOutput(tmpWallet, out)
		} else {
			out.Index = outWallet.AddrIndexMap(addr.String())
			e.outputManager.AddOutput(outWallet, out)
		}
	}
}

// updateInputWallet if input wallet is not specified, or aliases were provided without inputs (batch inputs) use Fresh faucet wallet.
func (e *EvilWallet) updateInputWallet(buildOptions *Options) error {
	for alias := range buildOptions.aliasInputs {
		// inputs provided for aliases (middle inputs in a batch)
		_, ok := e.aliasManager.GetInput(alias)
		if ok {
			// leave nil, wallet will be selected based on OutputIDWalletMap
			buildOptions.inputWallet = nil
			return nil
		}

		break
	}
	wallet, err := e.useFreshIfInputWalletNotProvided(buildOptions)
	if err != nil {
		return err
	}
	buildOptions.inputWallet = wallet

	return nil
}

func (e *EvilWallet) registerOutputAliases(signedTx *iotago.SignedTransaction, addrAliasMap map[string]string) {
	if len(addrAliasMap) == 0 {
		return
	}

	for idx := range signedTx.Transaction.Outputs {
		id := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), uint16(idx))
		out := e.outputManager.GetOutput(id)

		// register output alias
		e.aliasManager.AddOutputAlias(out, addrAliasMap[out.Address.String()])

		// register output as unspent output(input)
		e.aliasManager.AddInputAlias(out, addrAliasMap[out.Address.String()])
	}
}

func (e *EvilWallet) prepareInputs(buildOptions *Options) (inputs []*Output, err error) {
	if buildOptions.areInputsProvidedWithoutAliases() {
		inputs = append(inputs, buildOptions.inputs...)

		return
	}
	// append inputs with alias
	aliasInputs, err := e.matchInputsWithAliases(buildOptions)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, aliasInputs...)

	return inputs, nil
}

// prepareOutputs creates outputs for different scenarios, if no aliases were provided, new empty outputs are created from buildOptions.outputs balances.
func (e *EvilWallet) prepareOutputs(buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	addrAliasMap map[string]string, tempAddresses map[string]types.Empty, err error,
) {
	if buildOptions.areOutputsProvidedWithoutAliases() {
		outputs = append(outputs, buildOptions.outputs...)
	} else {
		// if outputs were provided with aliases
		outputs, addrAliasMap, tempAddresses, err = e.matchOutputsWithAliases(buildOptions, tempWallet)
	}

	return
}

// matchInputsWithAliases gets input from the alias manager. if input was not assigned to an alias before,
// it assigns a new Fresh faucet output.
func (e *EvilWallet) matchInputsWithAliases(buildOptions *Options) (inputs []*Output, err error) {
	// get inputs by alias
	for inputAlias := range buildOptions.aliasInputs {
		in, ok := e.aliasManager.GetInput(inputAlias)
		if !ok {
			wallet, err2 := e.useFreshIfInputWalletNotProvided(buildOptions)
			if err2 != nil {
				err = err2
				return
			}
			// No output found for given alias, use internal Fresh output if wallets are non-empty.
			in = e.wallets.GetUnspentOutput(wallet)
			if in == nil {
				return nil, ierrors.New("could not get unspent output")
			}
			e.aliasManager.AddInputAlias(in, inputAlias)
		}
		inputs = append(inputs, in)
	}

	return inputs, nil
}

func (e *EvilWallet) useFreshIfInputWalletNotProvided(buildOptions *Options) (*Wallet, error) {
	// if input wallet is not specified, use Fresh faucet wallet
	if buildOptions.inputWallet == nil {
		// deep spam enabled and no input reuse wallet provided, use evil wallet reuse wallet if enough outputs are available
		if buildOptions.reuse {
			outputsNeeded := len(buildOptions.inputs)
			if wallet := e.wallets.reuseWallet(outputsNeeded); wallet != nil {
				return wallet, nil
			}
		}

		wallet, err := e.wallets.freshWallet()
		if err != nil {
			return nil, ierrors.Wrap(err, "no Fresh wallet is available")
		}

		return wallet, nil
	}

	return buildOptions.inputWallet, nil
}

// matchOutputsWithAliases creates outputs based on balances provided via options.
// Outputs are not yet added to the Alias Manager, as they have no ID before the transaction is created.
// Thus, they are tracker in address to alias map. If the scenario is used, the outputBatchAliases map is provided
// that indicates which outputs should be saved to the outputWallet.All other outputs are created with temporary wallet,
// and their addresses are stored in tempAddresses.
func (e *EvilWallet) matchOutputsWithAliases(buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	addrAliasMap map[string]string, tempAddresses map[string]types.Empty, err error,
) {
	err = e.updateOutputBalances(buildOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	tempAddresses = make(map[string]types.Empty)
	addrAliasMap = make(map[string]string)
	for alias, output := range buildOptions.aliasOutputs {
		var addr *iotago.Ed25519Address
		if _, ok := buildOptions.outputBatchAliases[alias]; ok {
			addr = buildOptions.outputWallet.Address()
		} else {
			addr = tempWallet.Address()
			tempAddresses[addr.String()] = types.Void
		}

		outputs = append(outputs, &iotago.BasicOutput{
			Amount: output.BaseTokenAmount(),
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: addr},
			},
		})
		addrAliasMap[addr.String()] = alias
	}

	return
}

func (e *EvilWallet) prepareRemainderOutput(buildOptions *Options, outputs []iotago.Output) (alias string, remainderOutput iotago.Output, remainderAddress iotago.Address, added bool) {
	inputBalance := iotago.BaseToken(0)

	for inputAlias := range buildOptions.aliasInputs {
		in, _ := e.aliasManager.GetInput(inputAlias)
		inputBalance += in.Balance

		if alias == "" {
			remainderAddress = in.Address
			alias = inputAlias
		}
	}

	for _, input := range buildOptions.inputs {
		// get balance from output manager
		in := e.outputManager.GetOutput(input.OutputID)
		inputBalance += in.Balance

		if remainderAddress == nil {
			remainderAddress = in.Address
		}
	}

	outputBalance := iotago.BaseToken(0)
	for _, o := range outputs {
		outputBalance += o.BaseTokenAmount()
	}

	// remainder balances is sent to one of the address in inputs
	if outputBalance < inputBalance {
		remainderOutput = &iotago.BasicOutput{
			Amount: inputBalance - outputBalance,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: remainderAddress},
			},
		}

		added = true
	}

	return
}

func (e *EvilWallet) updateOutputBalances(buildOptions *Options) (err error) {
	// when aliases are not used for outputs, the balance had to be provided in options, nothing to do
	if buildOptions.areOutputsProvidedWithoutAliases() {
		return
	}
	totalBalance := iotago.BaseToken(0)
	if !buildOptions.isBalanceProvided() {
		if buildOptions.areInputsProvidedWithoutAliases() {
			for _, input := range buildOptions.inputs {
				// get balance from output manager
				inputDetails := e.outputManager.GetOutput(input.OutputID)
				totalBalance += inputDetails.Balance
			}
		} else {
			for inputAlias := range buildOptions.aliasInputs {
				in, ok := e.aliasManager.GetInput(inputAlias)
				if !ok {
					err = ierrors.New("could not get input by input alias")
					return
				}
				totalBalance += in.Balance
			}
		}
		balances := SplitBalanceEqually(len(buildOptions.outputs)+len(buildOptions.aliasOutputs), totalBalance)
		i := 0
		for out := range buildOptions.aliasOutputs {
			buildOptions.aliasOutputs[out] = &iotago.BasicOutput{
				Amount: balances[i],
			}
			i++
		}
	}

	return
}

func (e *EvilWallet) makeTransaction(inputs []*Output, outputs iotago.Outputs[iotago.Output], w *Wallet) (tx *iotago.SignedTransaction, err error) {
	clt := e.Connector().GetClient()

	txBuilder := builder.NewTransactionBuilder(clt.CurrentAPI())

	for _, input := range inputs {
		txBuilder.AddInput(&builder.TxInput{UnlockTarget: input.Address, InputID: input.OutputID, Input: input.OutputStruct})
	}

	for _, output := range outputs {
		txBuilder.AddOutput(output)
	}

	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	walletKeys := make([]iotago.AddressKeys, len(inputs))
	for i, input := range inputs {
		addr := input.Address
		var wallet *Wallet
		if w == nil { // aliases provided with inputs, use wallet saved in outputManager
			wallet = e.outputManager.OutputIDWalletMap(input.OutputID.ToHex())
		} else {
			wallet = w
		}
		index := wallet.AddrIndexMap(addr.String())
		inputPrivateKey, _ := wallet.KeyPair(index)
		walletKeys[i] = iotago.AddressKeys{Address: addr, Keys: inputPrivateKey}
	}
	txBuilder.SetCreationSlot(clt.CurrentAPI().TimeProvider().SlotFromTime(time.Now()))

	return txBuilder.Build(iotago.NewInMemoryAddressSigner(walletKeys...))
}

func (e *EvilWallet) PrepareCustomConflictsSpam(scenario *EvilScenario) (signedTxs [][]*iotago.SignedTransaction, allAliases ScenarioAlias, err error) {
	conflicts, allAliases := e.prepareConflictSliceForScenario(scenario)
	signedTxs, err = e.PrepareCustomConflicts(conflicts)

	return
}

func (e *EvilWallet) prepareConflictSliceForScenario(scenario *EvilScenario) (conflictSlice []ConflictSlice, allAliases ScenarioAlias) {
	genOutputOptions := func(aliases []string) []*OutputOption {
		outputOptions := make([]*OutputOption, 0)
		for _, o := range aliases {
			outputOptions = append(outputOptions, &OutputOption{aliasName: o})
		}

		return outputOptions
	}

	// make conflictSlice
	prefixedBatch, allAliases, batchOutputs := scenario.ConflictBatchWithPrefix()
	conflictSlice = make([]ConflictSlice, 0)
	for _, conflictMap := range prefixedBatch {
		conflicts := make([][]Option, 0)
		for _, aliases := range conflictMap {
			outs := genOutputOptions(aliases.Outputs)
			option := []Option{WithInputs(aliases.Inputs), WithOutputs(outs), WithOutputBatchAliases(batchOutputs)}
			if scenario.OutputWallet != nil {
				option = append(option, WithOutputWallet(scenario.OutputWallet))
			}
			if scenario.RestrictedInputWallet != nil {
				option = append(option, WithIssuer(scenario.RestrictedInputWallet))
			}
			if scenario.Reuse {
				option = append(option, WithReuseOutputs())
			}
			conflicts = append(conflicts, option)
		}
		conflictSlice = append(conflictSlice, conflicts)
	}

	return
}

// AwaitInputsSolidity waits for all inputs to be solid for client clt.
// func (e *EvilWallet) AwaitInputsSolidity(inputs devnetvm.Inputs, clt Client) (allSolid bool) {
// 	awaitSolid := make([]string, 0)
// 	for _, in := range inputs {
// 		awaitSolid = append(awaitSolid, in.Base58())
// 	}
// 	allSolid = e.outputManager.AwaitOutputsToBeSolid(awaitSolid, clt, maxGoroutines)
// 	return
// }

// SetTxOutputsSolid marks all outputs as solid in OutputManager for clientID.
func (e *EvilWallet) SetTxOutputsSolid(outputs iotago.OutputIDs, clientID string) {
	for _, out := range outputs {
		e.outputManager.SetOutputIDSolidForIssuer(out, clientID)
	}
}

// AddReuseOutputsToThePool adds all addresses corresponding to provided outputs to the reuse pool.
// func (e *EvilWallet) AddReuseOutputsToThePool(outputs devnetvm.Outputs) {
// 	for _, out := range outputs {
// 		evilOutput := e.outputManager.GetOutput(out.ID())
// 		if evilOutput != nil {
// 			wallet := e.outputManager.OutputIDWalletMap(out.ID().Base58())
// 			wallet.AddReuseAddress(evilOutput.Address.Base58())
// 		}
// 	}
// }

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

func WithFaucetSeed(seed []byte) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		copy(opts.optFaucetSeed[:], seed[:])
	}
}

func WithFaucetOutputID(id iotago.OutputID) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.optFaucetUnspentOutputID = id
	}
}

func WithClients(urls ...string) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.optsClientURLs = urls
	}
}
