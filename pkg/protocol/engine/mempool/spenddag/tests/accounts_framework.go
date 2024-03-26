package tests

import (
	"testing"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountsTestFramework struct {
	Instance  *account.Accounts
	Committee *account.SeatedAccounts

	test              *testing.T
	identitiesByAlias map[string]iotago.AccountID
}

func NewAccountsTestFramework(t *testing.T, instance *account.Accounts) *AccountsTestFramework {
	t.Helper()

	return &AccountsTestFramework{
		Instance:  instance,
		Committee: instance.SeatedAccounts(),

		test:              t,
		identitiesByAlias: make(map[string]iotago.AccountID),
	}
}

func (f *AccountsTestFramework) Add(alias string) {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(ierrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	f.Committee.Set(account.SeatIndex(f.Committee.SeatCount()), validatorID)
}

func (f *AccountsTestFramework) Delete(alias string) bool {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(ierrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.Delete(validatorID)
}

func (f *AccountsTestFramework) Get(alias string) (seat account.SeatIndex, exists bool) {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(ierrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.GetSeat(validatorID)
}

func (f *AccountsTestFramework) Has(alias string) bool {
	validatorID, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(ierrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return f.Committee.HasAccount(validatorID)
}

func (f *AccountsTestFramework) CreateID(alias string) iotago.AccountID {
	_, exists := f.identitiesByAlias[alias]
	if exists {
		f.test.Fatal(ierrors.Errorf("identity with alias '%s' already exists", alias))
	}

	hashedAlias := blake2b.Sum256([]byte(alias))
	validatorID := iotago.AccountIDFromData(hashedAlias[:])
	validatorID.RegisterAlias(alias)

	if err := f.Instance.Set(validatorID, &account.Pool{}); err != nil { // we don't care about pools when doing PoA
		f.test.Fatal(err)
	}
	f.Committee.Set(account.SeatIndex(f.Committee.SeatCount()), validatorID)

	f.identitiesByAlias[alias] = validatorID

	return validatorID
}

func (f *AccountsTestFramework) ID(alias string) iotago.AccountID {
	id, exists := f.identitiesByAlias[alias]
	if !exists {
		f.test.Fatal(ierrors.Errorf("identity with alias '%s' does not exist", alias))
	}

	return id
}
