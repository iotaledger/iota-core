package accountwallet

import (
	"github.com/iotaledger/hive.go/runtime/options"
)

// WithClientURL sets the client bind address.
func WithClientURL(url string) options.Option[AccountWallet] {
	return func(w *AccountWallet) {
		w.optsClientBindAddress = url
	}
}

func WithAccountStatesFile(fileName string) options.Option[AccountWallet] {
	return func(w *AccountWallet) {
		w.optsAccountStatesFile = fileName
	}
}

func WithFaucetAccountParams(params *faucetParams) options.Option[AccountWallet] {
	return func(w *AccountWallet) {
		w.optsFaucetParams = params
	}
}
