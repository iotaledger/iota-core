package types

type VM func(transaction Transaction, inputs []Output, gasLimit ...uint64) (outputs []Output, err error)
