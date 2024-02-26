package inspection

// Session is used to track instances of objects that have already been inspected.
type Session map[any]bool

// FirstOccurrence checks if the given instance has already been inspected.
func (s Session) FirstOccurrence(instance any) bool {
	if s[instance] {
		return false
	}

	s[instance] = true

	return true
}
