package model

import (
	"fmt"

	iotago "github.com/iotaledger/iota.go/v4"
)

// ParentsType is a type that defines the type of the parent.
type ParentsType uint8

const (
	// UndefinedParentType is the undefined parent.
	UndefinedParentType ParentsType = iota
	// StrongParentType is the ParentsType for a strong parent.
	StrongParentType
	// WeakParentType is the ParentsType for a weak parent.
	WeakParentType
	// ShallowLikeParentType is the ParentsType for the shallow like parent.
	ShallowLikeParentType
)

// String returns string representation of ParentsType.
func (p ParentsType) String() string {
	return fmt.Sprintf("ParentType(%s)", []string{"Undefined", "Strong", "Weak", "Shallow Like"}[p])
}

// Parent is a parent that can be either strong or weak.
type Parent struct {
	ID   iotago.BlockID
	Type ParentsType
}

// ParentReferences is a map between parent type and block IDs.
type ParentReferences map[ParentsType]iotago.BlockIDs
