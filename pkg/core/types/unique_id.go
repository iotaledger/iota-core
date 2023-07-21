package types

// UniqueID is a unique identifier.
type UniqueID uint64

// Next returns the next unique identifier.
func (u *UniqueID) Next() UniqueID {
	*u++

	return *u
}
