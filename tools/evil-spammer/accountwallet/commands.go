package accountwallet

import (
	"fmt"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (a *AccountWallet) CreateAccount(params *CreateAccountParams) (iotago.AccountID, error) {
	accountOutput, err := a.getFunds(params.Amount, iotago.AddressImplicitAccountCreation)
	if err != nil {
		return iotago.EmptyAccountID(), ierrors.Wrap(err, "Failed to create account")
	}

	accountID := iotago.AccountAddressFromOutputID(accountOutput.OutputID).AccountID()
	a.accountsAliases[params.Alias] = accountID
	a.aliasIndexMap[params.Alias] = a.latestUsedIndex
	fmt.Printf("Created account %s with %d tokens\n", accountID.ToHex(), params.Amount)

	return accountID, nil
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
