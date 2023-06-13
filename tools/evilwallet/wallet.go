package evilwallet

import (
	"crypto/ed25519"
	"sync"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"go.uber.org/atomic"
)

// region Wallet ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Wallet is the definition of a wallet.
type Wallet struct {
	ID                walletID
	walletType        WalletType
	unspentOutputs    map[string][]*Output // maps addr to its unspentOutput
	indexAddrMap      map[uint64]string
	addrIndexMap      map[string]uint64
	inputTransactions map[string]types.Empty
	reuseAddressPool  map[string]types.Empty
	seed              [32]byte

	lastAddrIdxUsed atomic.Int64 // used during filling in wallet with new outputs
	lastAddrSpent   atomic.Int64 // used during spamming with outputs one by one

	*sync.RWMutex
}

// NewWallet creates a wallet of a given type.
func NewWallet(wType ...WalletType) *Wallet {
	walletType := Other
	if len(wType) > 0 {
		walletType = wType[0]
	}
	idxSpent := atomic.NewInt64(-1)
	addrUsed := atomic.NewInt64(-1)

	wallet := &Wallet{
		walletType:        walletType,
		ID:                -1,
		seed:              tpkg.RandEd25519Seed(),
		unspentOutputs:    make(map[string][]*Output),
		indexAddrMap:      make(map[uint64]string),
		addrIndexMap:      make(map[string]uint64),
		inputTransactions: make(map[string]types.Empty),
		lastAddrSpent:     *idxSpent,
		lastAddrIdxUsed:   *addrUsed,
		RWMutex:           &sync.RWMutex{},
	}

	if walletType == Reuse {
		wallet.reuseAddressPool = make(map[string]types.Empty)
	}
	return wallet
}

// Type returns the wallet type.
func (w *Wallet) Type() WalletType {
	return w.walletType
}

// Address returns a new and unused address of a given wallet.
func (w *Wallet) Address() *iotago.Ed25519Address {
	w.Lock()
	defer w.Unlock()

	index := uint64(w.lastAddrIdxUsed.Add(1))
	hdWallet := mock.NewHDWallet("", w.seed[:], index)
	addr := hdWallet.Address()
	w.indexAddrMap[index] = addr.String()
	w.addrIndexMap[addr.String()] = index
	return addr
}

// Address returns a new and unused address of a given wallet.
func (w *Wallet) AddressOnIndex(index uint64) *iotago.Ed25519Address {
	w.Lock()
	defer w.Unlock()

	hdWallet := mock.NewHDWallet("", w.seed[:], index)
	addr := hdWallet.Address()
	return addr
}

// UnspentOutput returns the unspent output on the address.
func (w *Wallet) UnspentOutput(addr string) []*Output {
	w.RLock()
	defer w.RUnlock()

	return w.unspentOutputs[addr]
}

// UnspentOutputs returns all unspent outputs on the wallet.
func (w *Wallet) UnspentOutputs() (outputs map[string][]*Output) {
	w.RLock()
	defer w.RUnlock()
	outputs = make(map[string][]*Output)
	for addr, outs := range w.unspentOutputs {
		outputs[addr] = outs
	}
	return outputs
}

// IndexAddrMap returns the address for the index specified.
func (w *Wallet) IndexAddrMap(outIndex uint64) string {
	w.RLock()
	defer w.RUnlock()

	return w.indexAddrMap[outIndex]
}

// AddrIndexMap returns the index for the address specified.
func (w *Wallet) AddrIndexMap(address string) uint64 {
	w.RLock()
	defer w.RUnlock()

	return w.addrIndexMap[address]
}

// AddUnspentOutput adds an unspentOutput of a given wallet.
func (w *Wallet) AddUnspentOutput(addr *iotago.Ed25519Address, addrIdx uint64, outputID iotago.OutputID, balance uint64) *Output {
	w.Lock()
	defer w.Unlock()

	out := &Output{
		OutputID: outputID,
		Address:  addr,
		Index:    addrIdx,
		Balance:  balance,
	}
	w.unspentOutputs[addr.String()] = append(w.unspentOutputs[addr.String()], out)
	return out
}

// UnspentOutputBalance returns the balance on the unspent output sitting on the address specified.
func (w *Wallet) UnspentOutputBalance(addr string) uint64 {
	w.RLock()
	defer w.RUnlock()

	total := uint64(0)
	if outs, ok := w.unspentOutputs[addr]; ok {
		for _, o := range outs {
			total += o.Balance
		}
	}
	return total
}

// IsEmpty returns true if the wallet is empty.
func (w *Wallet) IsEmpty() (empty bool) {
	switch w.walletType {
	case Reuse:
		empty = len(w.reuseAddressPool) == 0
	default:
		empty = w.lastAddrSpent.Load() == w.lastAddrIdxUsed.Load() || w.UnspentOutputsLength() == 0
	}
	return
}

// UnspentOutputsLeft returns how many unused outputs are available in wallet.
func (w *Wallet) UnspentOutputsLeft() (left int) {
	switch w.walletType {
	case Reuse:
		left = len(w.reuseAddressPool)
	default:
		left = int(w.lastAddrIdxUsed.Load() - w.lastAddrSpent.Load())
	}
	return
}

// AddReuseAddress adds address to the reuse ready outputs' addresses pool for a Reuse wallet.
func (w *Wallet) AddReuseAddress(addr string) {
	w.Lock()
	defer w.Unlock()

	if w.walletType == Reuse {
		w.reuseAddressPool[addr] = types.Void
	}
}

// GetReuseAddress get random address from reuse addresses reuseOutputsAddresses pool. Address is removed from the pool after selecting.
func (w *Wallet) GetReuseAddress() string {
	w.Lock()
	defer w.Unlock()

	if w.walletType == Reuse {
		if len(w.reuseAddressPool) > 0 {
			for addr := range w.reuseAddressPool {
				delete(w.reuseAddressPool, addr)
				return addr
			}
		}
	}
	return ""
}

// GetUnspentOutput returns an unspent output on the oldest address ordered by index.
func (w *Wallet) GetUnspentOutput() *Output {
	switch w.walletType {
	case Reuse:
		addr := w.GetReuseAddress()
		return w.UnspentOutput(addr)[0]
	default:
		if w.lastAddrSpent.Load() < w.lastAddrIdxUsed.Load() {
			idx := w.lastAddrSpent.Add(1)
			addr := w.IndexAddrMap(uint64(idx))
			outs := w.UnspentOutput(addr)
			return outs[0]
		}
	}
	return nil
}

// Sign signs the tx essence.
func (w *Wallet) AddressSigner(addr *iotago.Ed25519Address) iotago.AddressSigner {
	w.RLock()
	defer w.RUnlock()
	index := w.AddrIndexMap(addr.String())
	hdWallet := mock.NewHDWallet("", w.seed[:], index)

	return hdWallet.AddressSigner()
}

func (w *Wallet) KeyPair(index uint64) (ed25519.PrivateKey, ed25519.PublicKey) {
	w.RLock()
	defer w.RUnlock()
	hdWallet := mock.NewHDWallet("", w.seed[:], index)

	return hdWallet.KeyPair()
}

// UpdateUnspentOutputID updates the unspent output on the address specified.
// func (w *Wallet) UpdateUnspentOutputID(addr string, outputID utxo.OutputID) error {
// 	w.RLock()
// 	walletOutput, ok := w.unspentOutputs[addr]
// 	w.RUnlock()
// 	if !ok {
// 		return errors.Errorf("could not find unspent output under provided address in the wallet, outID:%s, addr: %s", outputID.Base58(), addr)
// 	}
// 	w.Lock()
// 	walletOutput.OutputID = outputID
// 	w.Unlock()
// 	return nil
// }

// UnspentOutputsLength returns the number of unspent outputs on the wallet.
func (w *Wallet) UnspentOutputsLength() int {
	return len(w.unspentOutputs)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////
