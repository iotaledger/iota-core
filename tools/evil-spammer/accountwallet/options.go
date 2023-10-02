package accountwallet

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
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

func WithFaucetUnspendOutputID(id iotago.OutputID) options.Option[AccountWallet] {
	return func(w *AccountWallet) {
		w.optsFaucetUnspendOutputID = id
	}
}
