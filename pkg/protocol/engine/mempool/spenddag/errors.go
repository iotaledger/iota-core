package spenddag

import "github.com/iotaledger/hive.go/ierrors"

var (
	ErrExpected              = ierrors.New("expected error")
	ErrAlreadyPartOfSpendSet = ierrors.New("spender already part of SpendSet")
	ErrEntityEvicted         = ierrors.New("tried to operate on evicted entity")
	ErrFatal                 = ierrors.New("fatal error")
)
