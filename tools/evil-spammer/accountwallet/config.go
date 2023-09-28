package accountwallet

// commands

const (
	CreateAccountCommand  = "create"
	DestroyAccountCommand = "destroy"
	AllotAccountCommand   = "allot"
	ListAccountsCommand   = "list"
)

type configuration struct {
	WebAPI string `json:"WebAPI,omitempty"`
}

type CommandParams interface {
	CreateAccountParams | DestroyAccountParams | AllotAccountParams
}

type CreateAccountParams struct {
	Alias  string
	Amount uint64
}

type DestroyAccountParams struct {
	AccountAlias string
}

type AllotAccountParams struct {
	AccountAlias string
	Amount       uint64
}

const lockFile = "wallet.LOCK"

func loadWallet() *AccountWallet {
	return nil
}

func writeWalletStateFile(w any, filename string) {

}
