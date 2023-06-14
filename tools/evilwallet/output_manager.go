package evilwallet

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/types"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	awaitOutputsByAddress    = 150 * time.Second
	awaitOutputToBeConfirmed = 150 * time.Second
)

// Input contains details of an input.
type Input struct {
	OutputID iotago.OutputID
	Address  *iotago.Ed25519Address
}

// Output contains details of an output ID.
type Output struct {
	OutputID     iotago.OutputID
	Address      *iotago.Ed25519Address
	Index        uint64
	Balance      uint64
	CreationTime iotago.SlotIndex

	OutputStruct iotago.Output
}

// Outputs is a list of Output.
type Outputs []*Output

// OutputManager keeps track of the output statuses.
type OutputManager struct {
	connector Connector

	wallets           *Wallets
	outputIDWalletMap map[string]*Wallet
	outputIDAddrMap   map[string]string
	// stores solid outputs per node
	issuerSolidOutIDMap map[string]map[iotago.OutputID]types.Empty

	sync.RWMutex
}

// NewOutputManager creates an OutputManager instance.
func NewOutputManager(connector Connector, wallets *Wallets) *OutputManager {
	return &OutputManager{
		connector:           connector,
		wallets:             wallets,
		outputIDWalletMap:   make(map[string]*Wallet),
		outputIDAddrMap:     make(map[string]string),
		issuerSolidOutIDMap: make(map[string]map[iotago.OutputID]types.Empty),
	}
}

// setOutputIDWalletMap sets wallet for the provided outputID.
func (o *OutputManager) setOutputIDWalletMap(outputID string, wallet *Wallet) {
	o.Lock()
	defer o.Unlock()

	o.outputIDWalletMap[outputID] = wallet
}

// setOutputIDAddrMap sets address for the provided outputID.
func (o *OutputManager) setOutputIDAddrMap(outputID string, addr string) {
	o.Lock()
	defer o.Unlock()

	o.outputIDAddrMap[outputID] = addr
}

// OutputIDWalletMap returns wallet corresponding to the outputID stored in OutputManager.
func (o *OutputManager) OutputIDWalletMap(outputID string) *Wallet {
	o.RLock()
	defer o.RUnlock()

	return o.outputIDWalletMap[outputID]
}

// OutputIDAddrMap returns address corresponding to the outputID stored in OutputManager.
func (o *OutputManager) OutputIDAddrMap(outputID string) (addr string) {
	o.RLock()
	defer o.RUnlock()

	addr = o.outputIDAddrMap[outputID]
	return
}

// SetOutputIDSolidForIssuer sets solid flag for the provided outputID and issuer.
func (o *OutputManager) SetOutputIDSolidForIssuer(outputID iotago.OutputID, issuer string) {
	o.Lock()
	defer o.Unlock()

	if _, ok := o.issuerSolidOutIDMap[issuer]; !ok {
		o.issuerSolidOutIDMap[issuer] = make(map[iotago.OutputID]types.Empty)
	}
	o.issuerSolidOutIDMap[issuer][outputID] = types.Void
}

// IssuerSolidOutIDMap checks whether output was marked as solid for a given node.
func (o *OutputManager) IssuerSolidOutIDMap(issuer string, outputID iotago.OutputID) (isSolid bool) {
	o.RLock()
	defer o.RUnlock()

	if solidOutputs, ok := o.issuerSolidOutIDMap[issuer]; ok {
		if _, isSolid = solidOutputs[outputID]; isSolid {
			return
		}
	}
	return
}

// Track the confirmed statuses of the given outputIDs, it returns true if all of them are confirmed.
func (o *OutputManager) Track(outputIDs []iotago.OutputID) (allConfirmed bool) {
	var (
		wg                     sync.WaitGroup
		unconfirmedOutputFound atomic.Bool
	)

	for _, ID := range outputIDs {
		wg.Add(1)

		go func(id iotago.OutputID) {
			defer wg.Done()

			if !o.AwaitOutputToBeAccepted(id, awaitOutputToBeConfirmed) {
				unconfirmedOutputFound.Store(true)
			}
		}(ID)
	}
	wg.Wait()

	return !unconfirmedOutputFound.Load()
}

// CreateOutputFromAddress creates output, retrieves outputID, and adds it to the wallet.
// Provided address should be generated from provided wallet. Considers only first output found on address.
func (o *OutputManager) CreateOutputFromAddress(w *Wallet, addr *iotago.Ed25519Address, balance uint64, outputID iotago.OutputID, creationTime iotago.SlotIndex, outputStruct iotago.Output) *Output {
	index := w.AddrIndexMap(addr.String())
	out := &Output{
		Address:      addr,
		Index:        index,
		OutputID:     outputID,
		Balance:      balance,
		CreationTime: creationTime,
		OutputStruct: outputStruct,
	}
	w.AddUnspentOutput(out)
	o.setOutputIDWalletMap(outputID.ToHex(), w)
	o.setOutputIDAddrMap(outputID.ToHex(), addr.String())
	return out
}

// AddOutput adds existing output from wallet w to the OutputManager.
func (o *OutputManager) AddOutput(w *Wallet, output *Output) *Output {
	idx := w.AddrIndexMap(output.Address.String())
	out := &Output{
		Address:      output.Address,
		Index:        idx,
		OutputID:     output.OutputID,
		Balance:      output.Balance,
		CreationTime: output.CreationTime,
	}
	w.AddUnspentOutput(out)
	o.setOutputIDWalletMap(output.OutputID.ToHex(), w)
	o.setOutputIDAddrMap(output.OutputID.ToHex(), output.Address.String())

	return out
}

// GetOutput returns the Output of the given outputID.
// Firstly checks if output can be retrieved by outputManager from wallet, if not does an API call.
func (o *OutputManager) GetOutput(outputID iotago.OutputID) (output *Output) {
	output = o.getOutputFromWallet(outputID)

	// get output info via web api
	if output == nil {
		clt := o.connector.GetClient()
		out := clt.GetOutput(outputID)
		if out == nil {
			return nil
		}

		basicOutput, isBasic := out.(*iotago.BasicOutput)
		if !isBasic {
			return nil
		}

		output = &Output{
			OutputID:     outputID,
			Address:      basicOutput.UnlockConditionSet().Address().Address.(*iotago.Ed25519Address),
			Balance:      basicOutput.Deposit(),
			OutputStruct: basicOutput,
		}
	}

	return output
}

func (o *OutputManager) getOutputFromWallet(outputID iotago.OutputID) (output *Output) {
	o.RLock()
	defer o.RUnlock()
	w, ok := o.outputIDWalletMap[outputID.ToHex()]
	if ok {
		addr := o.outputIDAddrMap[outputID.ToHex()]
		output = w.UnspentOutput(addr)
	}
	return
}

// RequestOutputsByAddress finds the unspent outputs of a given address and updates the provided output status map.
// func (o *OutputManager) RequestOutputsByAddress(address string) (outputIDs []iotago.OutputID) {
// 	s := time.Now()
// 	clt := o.connector.GetClient()
// 	for ; time.Since(s) < awaitOutputsByAddress; time.Sleep(1 * time.Second) {
// 		outputIDs, err := clt.GetAddressUnspentOutputs(address)
// 		if err == nil && len(outputIDs) > 0 {
// 			return outputIDs
// 		}
// 	}

// 	return
// }

// RequestOutputsByTxID adds the outputs of a given transaction to the output status map.
func (o *OutputManager) RequestOutputsByTxID(txID iotago.TransactionID) (outputIDs iotago.OutputIDs) {
	clt := o.connector.GetClient()

	tx, err := clt.GetTransaction(txID)
	if err != nil {
		return
	}

	for index := range tx.Essence.Outputs {
		Id := iotago.OutputIDFromTransactionIDAndIndex(txID, uint16(index))
		outputIDs = append(outputIDs, Id)
	}

	return outputIDs
}

// AwaitWalletOutputsToBeConfirmed awaits for all outputs in the wallet are confirmed.
func (o *OutputManager) AwaitWalletOutputsToBeConfirmed(wallet *Wallet) {
	wg := sync.WaitGroup{}
	for _, output := range wallet.UnspentOutputs() {
		wg.Add(1)
		if output == nil {
			continue
		}

		var outs iotago.OutputIDs
		outs = append(outs, output.OutputID)

		go func(outs iotago.OutputIDs) {
			defer wg.Done()

			o.Track(outs)
		}(outs)
	}
	wg.Wait()
}

// AwaitOutputToBeAccepted awaits for output from a provided outputID is accepted. Timeout is waitFor.
// Useful when we have only an address and no transactionID, e.g. faucet funds request.
func (o *OutputManager) AwaitOutputToBeAccepted(outputID iotago.OutputID, waitFor time.Duration) (accepted bool) {
	s := time.Now()
	clt := o.connector.GetClient()
	accepted = false
	for ; time.Since(s) < waitFor; time.Sleep(awaitConfirmationSleep) {
		confirmationState := clt.GetOutputConfirmationState(outputID)
		if confirmationState == "confirmed" {
			accepted = true
			break
		}
	}

	return accepted
}

// AwaitTransactionsConfirmation awaits for transaction confirmation and updates wallet with outputIDs.
func (o *OutputManager) AwaitTransactionsConfirmation(txIDs iotago.TransactionIDs, maxGoroutines int) {
	wg := sync.WaitGroup{}
	semaphore := make(chan bool, maxGoroutines)

	for _, txID := range txIDs {
		wg.Add(1)
		go func(txID iotago.TransactionID) {
			defer wg.Done()
			semaphore <- true
			defer func() {
				<-semaphore
			}()
			err := o.AwaitTransactionToBeAccepted(txID, waitForConfirmation)
			if err != nil {
				return
			}
		}(txID)
	}
	wg.Wait()
}

// AwaitTransactionToBeAccepted awaits for acceptance of a single transaction.
func (o *OutputManager) AwaitTransactionToBeAccepted(txID iotago.TransactionID, waitFor time.Duration) error {
	s := time.Now()
	clt := o.connector.GetClient()
	var accepted bool
	for ; time.Since(s) < waitFor; time.Sleep(awaitConfirmationSleep) {
		// TODO: need to change to pending for now
		if confirmationState := clt.GetTransactionConfirmationState(txID); confirmationState == "pending" {
			accepted = true
			break
		}
	}
	if !accepted {
		return errors.Errorf("transaction %s not accepted in time", txID)
	}
	return nil
}

// AwaitOutputToBeSolid awaits for solidification of a single output by provided clt.
func (o *OutputManager) AwaitOutputToBeSolid(outID iotago.OutputID, clt Client, waitFor time.Duration) error {
	s := time.Now()
	var solid bool

	for ; time.Since(s) < waitFor; time.Sleep(awaitSolidificationSleep) {
		solid = o.IssuerSolidOutIDMap(clt.URL(), outID)
		if solid {
			break
		}
		if output := clt.GetOutput(outID); output != nil {
			o.SetOutputIDSolidForIssuer(outID, clt.URL())
			solid = true
			break
		}
	}
	if !solid {
		return errors.Errorf("output %s not solidified in time", outID)
	}
	return nil
}

// AwaitOutputsToBeSolid awaits for all provided outputs are solid for a provided client.
func (o *OutputManager) AwaitOutputsToBeSolid(outputs iotago.OutputIDs, clt Client, maxGoroutines int) (allSolid bool) {
	wg := sync.WaitGroup{}
	semaphore := make(chan bool, maxGoroutines)
	allSolid = true

	for _, outID := range outputs {
		wg.Add(1)
		go func(outID iotago.OutputID) {
			defer wg.Done()
			semaphore <- true
			defer func() {
				<-semaphore
			}()
			err := o.AwaitOutputToBeSolid(outID, clt, waitForSolidification)
			if err != nil {
				allSolid = false
				return
			}
		}(outID)
	}
	wg.Wait()
	return
}
