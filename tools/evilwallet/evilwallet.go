package evilwallet

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

const (
	// FaucetRequestSplitNumber defines the number of outputs to split from a faucet request.
	FaucetRequestSplitNumber = 100
	faucetTokensPerRequest   = 10_0000

	waitForConfirmation   = 150 * time.Second
	waitForSolidification = 150 * time.Second

	awaitConfirmationSleep   = 3 * time.Second
	awaitSolidificationSleep = time.Millisecond * 500

	WaitForTxSolid = 150 * time.Second

	maxGoroutines = 5
)

var (
	defaultClientsURLs   = []string{"http://localhost:8080", "http://localhost:8090"}
	faucetBalance        = uint64(100_0000_0000)
	dockerProtocolParams = func() *iotago.ProtocolParameters {
		options := snapshotcreator.NewOptions(presets.Docker...)
		return &options.ProtocolParameters
	}

	dockerFaucetSeed = func() []byte {
		genesisSeed, err := base58.Decode("7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih")
		if err != nil {
			log.Fatal(xerrors.Errorf("failed to decode base58 seed, using the default one: %w", err))
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
	optFaucetIndex           uint64
	optFaucetUnspentOutputID iotago.OutputID
	optsClientURLs           []string
	optsProtocolParams       *iotago.ProtocolParameters
}

// NewEvilWallet creates an EvilWallet instance.
func NewEvilWallet(opts ...options.Option[EvilWallet]) *EvilWallet {
	return options.Apply(&EvilWallet{
		wallets:                  NewWallets(),
		aliasManager:             NewAliasManager(),
		optFaucetSeed:            dockerFaucetSeed(),
		optFaucetIndex:           0,
		optFaucetUnspentOutputID: iotago.OutputIDFromTransactionIDAndIndex(iotago.TransactionID{}, 0),
		optsClientURLs:           defaultClientsURLs,
		optsProtocolParams:       dockerProtocolParams(),
	}, opts, func(w *EvilWallet) {
		connector := NewWebClients(w.optsClientURLs)
		w.connector = connector
		w.outputManager = NewOutputManager(connector, w.wallets)

		w.faucet = NewWallet()
		w.faucet.seed = [32]byte(w.optFaucetSeed)

		clt := w.connector.GetClient()
		faucetOutput := clt.GetOutput(w.optFaucetUnspentOutputID)
		w.faucet.AddUnspentOutput(&Output{
			Address:      w.faucet.AddressOnIndex(0),
			Index:        w.optFaucetIndex,
			OutputID:     w.optFaucetUnspentOutputID,
			Balance:      faucetOutput.Deposit(),
			CreationTime: iotago.SlotIndex(0),
			OutputStruct: faucetOutput,
		})
	})
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

	outputID, err := e.requestFaucetFunds(initWallet)
	if err != nil {
		return
	}

	if buildOptions.outputAliasName != "" {
		output := e.outputManager.GetOutput(outputID)
		e.aliasManager.AddInputAlias(output, buildOptions.outputAliasName)
	}

	return
}

// RequestFreshBigFaucetWallets creates n new wallets, each wallet is created from one faucet request and contains 10000 outputs.
func (e *EvilWallet) RequestFreshBigFaucetWallets(numberOfWallets int) {
	// channel to block the number of concurrent goroutines
	semaphore := make(chan bool, maxGoroutines)
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

// RequestFreshBigFaucetWallet creates a new wallet and fills the wallet with 10000 outputs created from funds
// requested from the Faucet.
func (e *EvilWallet) RequestFreshBigFaucetWallet() (err error) {
	initWallet := NewWallet()
	fmt.Println("Requesting funds from faucet...")

	receiveWallet := e.NewWallet(Fresh)

	err = e.requestAndSplitFaucetFunds(initWallet, receiveWallet)
	if err != nil {
		return
	}

	e.wallets.SetWalletReady(receiveWallet)

	return
}

// RequestFreshFaucetWallet creates a new wallet and fills the wallet with 100 outputs created from funds
// requested from the Faucet.
func (e *EvilWallet) RequestFreshFaucetWallet() (err error) {
	initWallet := NewWallet()
	receiveWallet := e.NewWallet(Fresh)
	err = e.requestAndSplitFaucetFunds(initWallet, receiveWallet)
	if err != nil {
		return
	}

	e.wallets.SetWalletReady(receiveWallet)

	return
}

func (e *EvilWallet) requestAndSplitFaucetFunds(initWallet, receiveWallet *Wallet) (err error) {
	_, err = e.requestFaucetFunds(initWallet)
	if err != nil {
		return err
	}
	// first split 1 to FaucetRequestSplitNumber outputs
	return e.splitOutputs(initWallet, receiveWallet)
}

func (e *EvilWallet) requestFaucetFunds(wallet *Wallet) (outputID iotago.OutputID, err error) {
	receiveAddr := wallet.Address()
	clt := e.connector.GetClient()

	faucetAddr := e.faucet.AddressOnIndex(e.optFaucetIndex)
	unspentFaucet := e.faucet.UnspentOutput(faucetAddr.String())
	remainderAmount := unspentFaucet.Balance - faucetTokensPerRequest

	txBuilder := builder.NewTransactionBuilder(e.optsProtocolParams.NetworkID())

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

	tx, err := txBuilder.Build(e.optsProtocolParams, e.faucet.AddressSigner(faucetAddr))
	if err != nil {
		return iotago.OutputID{}, err
	}

	// send transaction
	blkID, err := clt.PostTransaction(tx)
	if err != nil {
		fmt.Println("post faucet request failed", err)
		return iotago.OutputID{}, err
	}
	fmt.Println("post faucet request success", blkID)
	fmt.Println("faucet new outputID:", iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(tx.ID()), 1).ToHex())

	output := e.outputManager.CreateOutputFromAddress(wallet, receiveAddr, faucetTokensPerRequest, iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(tx.ID()), 0), tx.Essence.Outputs[0])

	e.faucet.AddUnspentOutput(&Output{
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(tx.ID()), 1),
		Address:      faucetAddr,
		Index:        0,
		Balance:      tx.Essence.Outputs[1].Deposit(),
		OutputStruct: tx.Essence.Outputs[1],
	})

	// track output in output manager and make sure it's confirmed
	ok := e.outputManager.Track([]iotago.OutputID{output.OutputID})
	if !ok {
		err = errors.New("not all outputs has been confirmed")
		return
	}
	outputID = output.OutputID

	return
}

// splitOutputs splits faucet input to 100 outputs.
func (e *EvilWallet) splitOutputs(inputWallet, outputWallet *Wallet) error {
	if inputWallet.IsEmpty() {
		return errors.New("inputWallet is empty")
	}

	addr := inputWallet.AddressOnIndex(0)
	input, outputs := e.handleInputOutputDuringSplitOutputs(FaucetRequestSplitNumber, inputWallet, outputWallet, addr.String())

	tx, err := e.CreateTransaction(WithInputs(input), WithOutputs(outputs), WithIssuer(inputWallet), WithOutputWallet(outputWallet))
	if err != nil {
		return err
	}

	_, err = e.connector.GetClient().PostTransaction(tx)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// wait txs to be confirmed
	e.outputManager.AwaitTransactionsConfirmation(iotago.TransactionIDs{lo.PanicOnErr(tx.ID())}, maxGoroutines)

	return nil
}

func (e *EvilWallet) handleInputOutputDuringSplitOutputs(splitNumber int, inputWallet, receiveWallet *Wallet, inputAddr string) (input *Output, outputs []*OutputOption) {
	evilInput := inputWallet.UnspentOutput(inputAddr)
	input = evilInput

	inputBalance := evilInput.Balance

	balances := SplitBalanceEqually(splitNumber, inputBalance)
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

func (e *EvilWallet) PrepareCustomConflicts(conflictsMaps []ConflictSlice) (conflictBatch [][]*iotago.Transaction, err error) {
	for _, conflictMap := range conflictsMaps {
		var txs []*iotago.Transaction
		for _, options := range conflictMap {
			tx, err2 := e.CreateTransaction(options...)
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
			return errors.New("insufficient clients to send conflicts")
		}

		// send transactions in parallel
		wg := sync.WaitGroup{}
		for i, tx := range txs {
			wg.Add(1)
			go func(clt Client, tx *iotago.Transaction) {
				defer wg.Done()
				clt.PostTransaction(tx)
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
func (e *EvilWallet) CreateTransaction(options ...Option) (tx *iotago.Transaction, err error) {
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
			addrAliasMap[*remainderAddr] = alias
		}
	}

	tx, err = e.makeTransaction(inputs, outputs, buildOptions.inputWallet)
	if err != nil {
		return nil, err
	}

	e.addOutputsToOutputManager(tx, buildOptions.outputWallet, tempWallet, tempAddresses)
	e.registerOutputAliases(tx, addrAliasMap)

	return
}

// addOutputsToOutputManager adds output to the OutputManager if.
func (e *EvilWallet) addOutputsToOutputManager(tx *iotago.Transaction, outWallet, tmpWallet *Wallet, tempAddresses map[iotago.Ed25519Address]types.Empty) {
	for idx, o := range tx.Essence.Outputs {
		addr := o.UnlockConditionSet().Address().Address.(*iotago.Ed25519Address)
		out := &Output{
			OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(tx.ID()), uint16(idx)),
			Address:      addr,
			Balance:      o.Deposit(),
			OutputStruct: o}

		if _, ok := tempAddresses[*addr]; ok {
			e.outputManager.AddOutput(tmpWallet, out)
		} else {
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

func (e *EvilWallet) registerOutputAliases(tx *iotago.Transaction, addrAliasMap map[iotago.Ed25519Address]string) {
	if len(addrAliasMap) == 0 {
		return
	}

	for idx, output := range tx.Essence.Outputs {
		addr := output.UnlockConditionSet().Address().Address.(*iotago.Ed25519Address)
		id := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(tx.ID()), uint16(idx))
		out := &Output{
			OutputID:     id,
			Address:      addr,
			Balance:      output.Deposit(),
			OutputStruct: output,
		}
		// register output alias
		e.aliasManager.AddOutputAlias(out, addrAliasMap[*addr])

		// register output as unspent output(input)
		e.aliasManager.AddInputAlias(out, addrAliasMap[*addr])
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
	addrAliasMap map[iotago.Ed25519Address]string, tempAddresses map[iotago.Ed25519Address]types.Empty, err error,
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
			out := e.wallets.GetUnspentOutput(wallet)
			if out == nil {
				return nil, errors.New("could not get unspent output")
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
		if wallet, err := e.wallets.freshWallet(); wallet != nil {
			return wallet, nil
		} else {
			return nil, errors.Wrap(err, "no Fresh wallet is available")
		}
	}
	return buildOptions.inputWallet, nil
}

// matchOutputsWithAliases creates outputs based on balances provided via options.
// Outputs are not yet added to the Alias Manager, as they have no ID before the transaction is created.
// Thus, they are tracker in address to alias map. If the scenario is used, the outputBatchAliases map is provided
// that indicates which outputs should be saved to the outputWallet.All other outputs are created with temporary wallet,
// and their addresses are stored in tempAddresses.
func (e *EvilWallet) matchOutputsWithAliases(buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	addrAliasMap map[iotago.Ed25519Address]string, tempAddresses map[iotago.Ed25519Address]types.Empty, err error) {
	err = e.updateOutputBalances(buildOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	tempAddresses = make(map[iotago.Ed25519Address]types.Empty)
	addrAliasMap = make(map[iotago.Ed25519Address]string)
	for alias, output := range buildOptions.aliasOutputs {
		var addr iotago.Ed25519Address
		if _, ok := buildOptions.outputBatchAliases[alias]; ok {
			addr = *buildOptions.outputWallet.Address()
		} else {
			addr = *tempWallet.Address()
			tempAddresses[addr] = types.Void
		}

		outputs = append(outputs, output)
		addrAliasMap[addr] = alias
	}

	return
}

func (e *EvilWallet) prepareRemainderOutput(buildOptions *Options, outputs []iotago.Output) (alias string, remainderOutput iotago.Output, remainderAddress *iotago.Ed25519Address, added bool) {
	inputBalance := uint64(0)

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

	outputBalance := uint64(0)
	for _, o := range outputs {
		if o.Type() == iotago.OutputBasic {
			bo := o.(*iotago.BasicOutput)
			outputBalance += bo.Deposit()
		}
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
	totalBalance := uint64(0)
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
					err = errors.New("could not get input by input alias")
					return
				}
				output := e.outputManager.GetOutput(in.OutputID)
				totalBalance += output.Balance
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

func (e *EvilWallet) makeTransaction(inputs []*Output, outputs iotago.Outputs[iotago.Output], w *Wallet) (tx *iotago.Transaction, err error) {

	txBuilder := builder.NewTransactionBuilder(e.optsProtocolParams.NetworkID())

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

	return txBuilder.Build(e.optsProtocolParams, iotago.NewInMemoryAddressSigner(walletKeys...))
}

func (e *EvilWallet) getAddressFromInput(input *Output) (addr *iotago.Ed25519Address, err error) {
	out := e.outputManager.GetOutput(input.OutputID)
	if out == nil {
		err = errors.New("output not found in output manager")
		return
	}

	addr = out.Address

	return
}

func (e *EvilWallet) PrepareCustomConflictsSpam(scenario *EvilScenario) (txs [][]*iotago.Transaction, allAliases ScenarioAlias, err error) {
	conflicts, allAliases := e.prepareConflictSliceForScenario(scenario)
	txs, err = e.PrepareCustomConflicts(conflicts)

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

func WithFaucetIndex(index uint64) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.optFaucetIndex = index
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

func WithProtoParams(protoParams *iotago.ProtocolParameters) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.optsProtocolParams = protoParams
	}
}
