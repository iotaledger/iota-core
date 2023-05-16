package promise

// UniqueID is an identifier for a resource.
type UniqueID uint64

func (u *UniqueID) Next() UniqueID {
	*u++

	return *u
}
