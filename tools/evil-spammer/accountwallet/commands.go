package accountwallet

import (
	"fmt"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (a *AccountWallet) CreateAccount(params *CreateAccountParams) (iotago.AccountID, error) {
	implicitAccountOutput, privateKey, err := a.getFunds(params.Amount, iotago.AddressImplicitAccountCreation)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "Failed to create account")
	}

	accountID := a.registerAccount(params.Alias, implicitAccountOutput.OutputID, a.latestUsedIndex, privateKey)

	fmt.Printf("Created account %s with %d tokens\n", accountID.ToHex(), params.Amount)

	return accountID, nil
}

func (a *AccountWallet) DestroyAccount(params *DestroyAccountParams) error {
	return a.destroyAccount(params.AccountAlias)
}

func (a *AccountWallet) ListAccount() error {
	fmt.Printf("%-10s \t%-33s\n\n", "Alias", "AccountID")
	for _, accData := range a.accountsAliases {
		fmt.Printf("%-10s \t", accData.Alias)
		fmt.Printf("%-33s ", accData.Account.ID().ToHex())
		fmt.Printf("\n")
	}

	return nil
}

func (a *AccountWallet) AllotToAccount(params *AllotAccountParams) error {
	return nil
}
