package programs

import (
	"encoding/json"
	"os"
	"time"

	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
)

type CustomSpamParams struct {
	ClientURLs            []string
	SpamTypes             []string
	Rates                 []int
	Durations             []time.Duration
	BlkToBeSent           []int
	TimeUnit              time.Duration
	DelayBetweenConflicts time.Duration
	NSpend                int
	Scenario              wallet.EvilBatch
	DeepSpam              bool
	EnableRateSetter      bool
	AccountAlias          string

	Config *BasicConfig
}

type BasicConfig struct {
	LastFaucetUnspentOutputID string `json:"lastFaucetUnspentOutputId"`
}

var basicConfigFile = "basic_config.json"

var basicConfigJSON = `{
	"lastFaucetUnspentOutputId": ""
}`

// LoadBasicConfig loads the config file.
func LoadBasicConfig() *BasicConfig {
	// open config file
	config := new(BasicConfig)
	file, err := os.Open(basicConfigFile)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}

		//nolint:gosec // users should be able to read the file
		if err = os.WriteFile(basicConfigFile, []byte(basicConfigJSON), 0o644); err != nil {
			panic(err)
		}
		if file, err = os.Open(basicConfigFile); err != nil {
			panic(err)
		}
	}
	defer file.Close()

	// decode config file
	if err = json.NewDecoder(file).Decode(config); err != nil {
		panic(err)
	}

	return config
}

func SaveConfigsToFile(config *BasicConfig) {
	// open config file
	file, err := os.Open(basicConfigFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	jsonConfigs, err := json.MarshalIndent(config, "", "    ")

	if err != nil {
		log.Errorf("failed to write configs to file %s", err)
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(basicConfigFile, jsonConfigs, 0o644); err != nil {
		panic(err)
	}
}
