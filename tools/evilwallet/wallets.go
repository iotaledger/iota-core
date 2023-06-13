package evilwallet

import (
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/atomic"
)

type walletID int

type (
	// WalletType is the type of the wallet.
	WalletType   int8
	WalletStatus int8
)

const (
	Other WalletType = iota
	// Fresh is used for automatic Faucet Requests, outputs are returned one by one.
	Fresh
	// Reuse stores resulting outputs of double spends or transactions issued by the evilWallet,
	// outputs from this wallet are reused in spamming scenario with flag reuse set to true and no RestrictedReuse wallet provided.
	// Reusing spammed outputs allows for creation of deeper UTXO DAG structure.
	Reuse
	// RestrictedReuse it is a reuse wallet, that will be available only to spamming scenarios that
	// will provide RestrictedWallet for the reuse spamming.
	RestrictedReuse
)

// region Wallets ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Wallets is a container of wallets.
type Wallets struct {
	wallets map[walletID]*Wallet
	// we store here non-empty wallets ids of wallets with Fresh faucet outputs.
	faucetWallets []walletID
	// reuse wallets are stored without an order, so they are picked up randomly.
	// Boolean flag indicates if wallet is ready - no new addresses will be generated, so empty wallets can be deleted.
	reuseWallets map[walletID]bool
	mu           sync.RWMutex

	lastWalletID atomic.Int64
}

// NewWallets creates and returns a new Wallets container.
func NewWallets() *Wallets {
	return &Wallets{
		wallets:       make(map[walletID]*Wallet),
		faucetWallets: make([]walletID, 0),
		reuseWallets:  make(map[walletID]bool),
		lastWalletID:  *atomic.NewInt64(-1),
	}
}

// NewWallet adds a new wallet to Wallets and returns the created wallet.
func (w *Wallets) NewWallet(walletType WalletType) *Wallet {
	wallet := NewWallet(walletType)
	wallet.ID = walletID(w.lastWalletID.Add(1))

	w.addWallet(wallet)
	if walletType == Reuse {
		w.addReuseWallet(wallet)
	}
	return wallet
}

// GetWallet returns the wallet with the specified ID.
func (w *Wallets) GetWallet(walletID walletID) *Wallet {
	return w.wallets[walletID]
}

// GetNextWallet get next non-empty wallet based on provided type.
func (w *Wallets) GetNextWallet(walletType WalletType, minOutputsLeft int) (*Wallet, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch walletType {
	case Fresh:
		if !w.IsFaucetWalletAvailable() {
			return nil, errors.New("no faucet wallets available, need to request more funds")
		}

		wallet := w.wallets[w.faucetWallets[0]]
		if wallet.IsEmpty() {
			return nil, errors.New("wallet is empty, need to request more funds")
		}
		return wallet, nil
	case Reuse:
		for id, ready := range w.reuseWallets {
			wal := w.wallets[id]
			if wal.UnspentOutputsLeft() > minOutputsLeft {
				// if are solid

				return wal, nil
			}
			// no outputs will be ever added to this wallet, so it can be deleted
			if wal.IsEmpty() && ready {
				w.removeReuseWallet(id)
			}
		}
		return nil, errors.New("no reuse wallets available")
	}

	return nil, errors.New("wallet type not supported for ordered usage, use GetWallet by ID instead")
}

func (w *Wallets) UnspentOutputsLeft(walletType WalletType) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	outputsLeft := 0

	switch walletType {
	case Fresh:
		for _, wID := range w.faucetWallets {
			outputsLeft += w.wallets[wID].UnspentOutputsLeft()
		}
	case Reuse:
		for wID := range w.reuseWallets {
			outputsLeft += w.wallets[wID].UnspentOutputsLeft()
		}
	}
	return outputsLeft
}

// addWallet stores newly created wallet.
func (w *Wallets) addWallet(wallet *Wallet) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.wallets[wallet.ID] = wallet
}

// addReuseWallet stores newly created wallet.
func (w *Wallets) addReuseWallet(wallet *Wallet) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.reuseWallets[wallet.ID] = false
}

// GetUnspentOutput gets first found unspent output for a given walletType.
func (w *Wallets) GetUnspentOutput(wallet *Wallet) *Output {
	if wallet == nil {
		return nil
	}
	return wallet.GetUnspentOutput()
}

// IsFaucetWalletAvailable checks if there is any faucet wallet left.
func (w *Wallets) IsFaucetWalletAvailable() bool {
	return len(w.faucetWallets) > 0
}

// freshWallet returns the first non-empty wallet from the faucetWallets queue. If current wallet is empty,
// it is removed and the next enqueued one is returned.
func (w *Wallets) freshWallet() (*Wallet, error) {
	wallet, err := w.GetNextWallet(Fresh, 1)
	if err != nil {
		w.removeFreshWallet()
		wallet, err = w.GetNextWallet(Fresh, 1)
		if err != nil {
			return nil, err
		}
	}
	return wallet, nil
}

// reuseWallet returns the first non-empty wallet from the reuseWallets queue. If current wallet is empty,
// it is removed and the next enqueued one is returned.
func (w *Wallets) reuseWallet(outputsNeeded int) *Wallet {
	wallet, err := w.GetNextWallet(Reuse, outputsNeeded)
	if err != nil {
		return nil
	}
	return wallet
}

// removeWallet removes wallet, for Fresh wallet it will be the first wallet in a queue.
func (w *Wallets) removeFreshWallet() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.IsFaucetWalletAvailable() {
		removedID := w.faucetWallets[0]
		w.faucetWallets = w.faucetWallets[1:]
		delete(w.wallets, removedID)
	}
}

// removeWallet removes wallet, for Fresh wallet it will be the first wallet in a queue.
func (w *Wallets) removeReuseWallet(walletID walletID) {
	if _, ok := w.reuseWallets[walletID]; ok {
		delete(w.reuseWallets, walletID)
		delete(w.wallets, walletID)
	}
}

// SetWalletReady makes wallet ready to use, Fresh wallet is added to freshWallets queue.
func (w *Wallets) SetWalletReady(wallet *Wallet) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if wallet.IsEmpty() {
		return
	}
	wType := wallet.walletType
	switch wType {
	case Fresh:
		w.faucetWallets = append(w.faucetWallets, wallet.ID)
	case Reuse:
		w.reuseWallets[wallet.ID] = true
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////
