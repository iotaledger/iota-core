package vote

// MockedRank is a mocked rank implementation that is used for testing.
type MockedRank int

// Compare compares the MockedRank to another MockedRank.
func (m MockedRank) Compare(other MockedRank) int {
	switch {
	case m < other:
		return -1
	case m > other:
		return 1
	default:
		return 0
	}
}
