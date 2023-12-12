package utxoledger

// Denotes the type of state.
type StateType byte

const (
	StateTypeUTXOInput StateType = iota
	StateTypeCommitment
)
