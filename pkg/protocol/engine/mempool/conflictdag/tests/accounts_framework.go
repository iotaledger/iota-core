package tests

import (
	"testing"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountsTestFramework struct {
	Instance  *account.Accounts[iotago.AccountID, *iotago.AccountID]
	Committee *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID]

	test              *testing.T
	identitiesByAlias map[string]iotago.AccountID
}

func NewAccountsTestFramework(test *testing.T, instance *account.Accounts[iotago.AccountID, *iotago.AccountID]) *AccountsTestFramework {
	return &AccountsTestFramework{
		Instance:  instance,
		Committee: instance.SelectAccounts(),

		test:              test,
		identitiesByAlias: make(map[string]iotago.AccountID),
	}
}

func (f *AccountsTestFramework) Add(alias string) bool {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(xerrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.Add(validatorID)
}

func (f *AccountsTestFramework) Delete(alias string) bool {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(xerrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.Delete(validatorID)
}

func (f *AccountsTestFramework) Get(alias string) (weight int64, exists bool) {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(xerrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.Get(validatorID)
}

func (f *AccountsTestFramework) Has(alias string) bool {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(xerrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.Has(validatorID)
}

func (f *AccountsTestFramework) ForEachWeighted(callback func(id iotago.AccountID, weight int64) bool) error {
	return f.Instance.ForEach(callback)
}

func (f *AccountsTestFramework) TotalWeight() int64 {
	return f.Instance.TotalWeight()
}

func (f *AccountsTestFramework) CreateID(alias string, optWeight ...int64) iotago.AccountID {
	validatorID, exists := f.identitiesByAlias[alias]
	if exists {
		f.test.Fatal(xerrors.Errorf("identity with alias '%s' already exists", alias))
	}

	hashedAlias := blake2b.Sum256([]byte(alias))
	validatorID = iotago.IdentifierFromData(hashedAlias[:])
	validatorID.RegisterAlias(alias)

	f.Instance.Set(validatorID, lo.First(optWeight))
	f.Committee.Add(validatorID)

	f.identitiesByAlias[alias] = validatorID

	return validatorID
}

func (f *AccountsTestFramework) ID(alias string) iotago.AccountID {
	id, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(xerrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return id
}
