package mempool

import (
	"context"
)

type TransactionValidator func(signedTransaction SignedTransaction, resolvedInputs []State) error

type TransactionExecutor func(executionContext context.Context, transaction Transaction) (outputs []State, err error)
