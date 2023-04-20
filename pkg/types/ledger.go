package types

type Ledger interface {
	Output(id OutputID) (output Output, exists bool)
}
