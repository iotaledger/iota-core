package wallet

import iotago "github.com/iotaledger/iota.go/v4"

// region utxo/tx realted functions ////////////////////////////////////////////////////////////////////////////////////////////

// SplitBalanceEqually splits the balance equally between `splitNumber` outputs.
func SplitBalanceEqually(splitNumber int, balance iotago.BaseToken) []iotago.BaseToken {
	outputBalances := make([]iotago.BaseToken, 0)

	// make sure the output balances are equal input
	var totalBalance iotago.BaseToken

	// input is divided equally among outputs
	for i := 0; i < splitNumber-1; i++ {
		outputBalances = append(outputBalances, balance/iotago.BaseToken(splitNumber))
		totalBalance += outputBalances[i]
	}
	lastBalance := balance - totalBalance
	outputBalances = append(outputBalances, lastBalance)

	return outputBalances
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////////
