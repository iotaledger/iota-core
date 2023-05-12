package conflictdagv1

import (
	"testing"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/tests"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TestConflictDAG runs the generic tests for the ConflictDAG.
func TestConflictDAG(t *testing.T) {
	tests.TestAll(t, newTestFramework)
}

// newTestFramework creates a new instance of the TestFramework for internal unit tests.
func newTestFramework(t *testing.T) *tests.Framework[iotago.TransactionID, iotago.OutputID, vote.MockedPower] {
	accountsTestFramework := tests.NewAccountsTestFramework(t, account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()))

	return tests.NewFramework[iotago.TransactionID, iotago.OutputID, vote.MockedPower](
		t,
		New[iotago.TransactionID, iotago.OutputID, vote.MockedPower](accountsTestFramework.Committee),
		accountsTestFramework,
		transactionID,
		outputID,
	)
}

// transactionID creates a (made up) TransactionID from the given alias.
func transactionID(alias string) iotago.TransactionID {
	hashedAlias := blake2b.Sum256([]byte(alias))

	var result iotago.TransactionID
	_ = lo.PanicOnErr(result.FromBytes(hashedAlias[:]))

	result.RegisterAlias(alias)

	return result
}

// outputID creates a (made up) OutputID from the given alias.
func outputID(alias string) iotago.OutputID {
	hashedAlias := blake2b.Sum256([]byte(alias))

	return iotago.OutputIDFromTransactionIDAndIndex(iotago.IdentifierFromData(hashedAlias[:]), 1)
}
