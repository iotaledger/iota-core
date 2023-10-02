package accountwallet

import (
	"fmt"

	iotago "github.com/iotaledger/iota.go/v4"
)

func (a *AccountWallet) CreateAccount(params *CreateAccountParams) error {
	out := a.getFunds(params.Amount, iotago.AddressImplicitAccountCreation)
	fmt.Printf("Created account %s with %d tokens\n", out.Address, params.Amount)

	return nil
}

func (a *AccountWallet) DestroyAccount(params *DestroyAccountParams) error {
	return nil
}

func (a *AccountWallet) ListAccount() error {
	return nil
}

func (a *AccountWallet) AllotToAccount(params *AllotAccountParams) error {
	return nil
}
