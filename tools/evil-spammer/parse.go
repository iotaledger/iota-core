package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iotaledger/iota-core/tools/evil-spammer/accountwallet"
	"github.com/iotaledger/iota-core/tools/evil-spammer/programs"
	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
	iotago "github.com/iotaledger/iota.go/v4"
)

func parseFlags() (help bool) {
	if len(os.Args) <= 1 {
		return true
	}
	script := os.Args[1]

	Script = script
	log.Infof("script %s", Script)

	switch Script {
	case "basic":
		parseBasicSpamFlags()
	case "accounts":
		// pass subcommands
		subcommands := make([]string, 0)
		if len(os.Args) > 2 {
			subcommands = os.Args[2:]
		}
		accountsSubcommandsFlags = readSubcommandsAndFlagSets(subcommands)
		basicConfig := programs.LoadBasicConfig()
		outputID, err := iotago.OutputIDFromHex(basicConfig.LastFaucetUnspentOutputID)
		if err != nil {
			log.Warnf("Cannot parse faucet output id from config: %v", err)
		}
		lastFaucetUnspendOutputID = outputID
	case "quick":
		parseQuickTestFlags()
		// case SpammerTypeCommitments:
		// 	parseCommitmentsSpamFlags()
	}
	if Script == "help" || Script == "-h" || Script == "--help" {
		return true
	}

	return
}

func parseOptionFlagSet(flagSet *flag.FlagSet, args ...[]string) {
	commands := os.Args[2:]
	if len(args) > 0 {
		commands = args[0]
	}
	err := flagSet.Parse(commands)
	if err != nil {
		log.Errorf("Cannot parse first `script` parameter")
		return
	}
}

func parseBasicSpamFlags() {
	urls := optionFlagSet.String("urls", "", "API urls for clients used in test separated with commas")
	spamTypes := optionFlagSet.String("spammer", "", "Spammers used during test. Format: strings separated with comma, available options: 'blk' - block,"+
		" 'tx' - transaction, 'ds' - double spends spammers, 'nds' - n-spends spammer, 'custom' - spams with provided scenario")
	rate := optionFlagSet.String("rate", "", "Spamming rate for provided 'spammer'. Format: numbers separated with comma, e.g. 10,100,1 if three spammers were provided for 'spammer' parameter.")
	duration := optionFlagSet.String("duration", "", "Spam duration. Cannot be combined with flag 'blkNum'. Format: separated by commas list of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
	blkNum := optionFlagSet.String("blkNum", "", "Spam duration in seconds. Cannot be combined with flag 'duration'. Format: numbers separated with comma, e.g. 10,100,1 if three spammers were provided for 'spammer' parameter.")
	timeunit := optionFlagSet.Duration("tu", customSpamParams.TimeUnit, "Time unit for the spamming rate. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
	delayBetweenConflicts := optionFlagSet.Duration("dbc", customSpamParams.DelayBetweenConflicts, "delayBetweenConflicts - Time delay between conflicts in double spend spamming")
	scenario := optionFlagSet.String("scenario", "", "Name of the EvilBatch that should be used for the spam. By default uses Scenario1. Possible scenarios can be found in evilwallet/customscenarion.go.")
	deepSpam := optionFlagSet.Bool("deep", customSpamParams.DeepSpam, "Enable the deep spam, by reusing outputs created during the spam.")
	nSpend := optionFlagSet.Int("nSpend", customSpamParams.NSpend, "Number of outputs to be spent in n-spends spammer for the spammer type needs to be set to 'ds'. Default value is 2 for double-spend.")
	account := optionFlagSet.String("account", "", "Account alias to be used for the spam. Account should be created first with accounts tool.")

	parseOptionFlagSet(optionFlagSet)

	if *urls != "" {
		parsedUrls := parseCommaSepString(*urls)
		quickTestParams.ClientURLs = parsedUrls
		customSpamParams.ClientURLs = parsedUrls
	}
	if *spamTypes != "" {
		parsedSpamTypes := parseCommaSepString(*spamTypes)
		customSpamParams.SpamTypes = parsedSpamTypes
	}
	if *rate != "" {
		parsedRates := parseCommaSepInt(*rate)
		customSpamParams.Rates = parsedRates
	}
	if *duration != "" {
		parsedDurations := parseDurations(*duration)
		customSpamParams.Durations = parsedDurations
	}
	if *blkNum != "" {
		parsedBlkNums := parseCommaSepInt(*blkNum)
		customSpamParams.BlkToBeSent = parsedBlkNums
	}
	if *scenario != "" {
		conflictBatch, ok := wallet.GetScenario(*scenario)
		if ok {
			customSpamParams.Scenario = conflictBatch
		}
	}

	customSpamParams.NSpend = *nSpend
	customSpamParams.DeepSpam = *deepSpam
	customSpamParams.TimeUnit = *timeunit
	customSpamParams.DelayBetweenConflicts = *delayBetweenConflicts
	customSpamParams.AccountAlias = *account

	// fill in unused parameter: blkNum or duration with zeros
	if *duration == "" && *blkNum != "" {
		customSpamParams.Durations = make([]time.Duration, len(customSpamParams.BlkToBeSent))
	}
	if *blkNum == "" && *duration != "" {
		customSpamParams.BlkToBeSent = make([]int, len(customSpamParams.Durations))
	}

	customSpamParams.Config = programs.LoadBasicConfig()
}

func parseQuickTestFlags() {
	urls := optionFlagSet.String("urls", "", "API urls for clients used in test separated with commas")
	rate := optionFlagSet.Int("rate", quickTestParams.Rate, "The spamming rate")
	duration := optionFlagSet.Duration("duration", quickTestParams.Duration, "Duration of the spam. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
	timeunit := optionFlagSet.Duration("tu", quickTestParams.TimeUnit, "Time unit for the spamming rate. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
	delayBetweenConflicts := optionFlagSet.Duration("dbc", quickTestParams.DelayBetweenConflicts, "delayBetweenConflicts - Time delay between conflicts in double spend spamming")
	verifyLedger := optionFlagSet.Bool("verify", quickTestParams.VerifyLedger, "Set to true if verify ledger script should be run at the end of the test")

	parseOptionFlagSet(optionFlagSet)

	if *urls != "" {
		parsedUrls := parseCommaSepString(*urls)
		quickTestParams.ClientURLs = parsedUrls
	}
	quickTestParams.Rate = *rate
	quickTestParams.Duration = *duration
	quickTestParams.TimeUnit = *timeunit
	quickTestParams.DelayBetweenConflicts = *delayBetweenConflicts
	quickTestParams.VerifyLedger = *verifyLedger
}

type subcommand struct {
	command string
	flags   []string
}

// readSubcommandsAndFlagSets splits the subcommands on multiple flag sets.
func readSubcommandsAndFlagSets(subcommands []string) []*subcommand {
	prevSplitIndex := 0
	subcommandsSplit := make([]*subcommand, 0)
	if len(subcommands) == 0 {
		accountUsage()

		return nil
	}
	for index := 0; index < len(subcommands); index++ {
		_, validCommand := accountwallet.AvailableCommands[subcommands[index]]
		if subcommands[index] == "-h" || subcommands[index] == "--help" {
			accountUsage()

			return nil
		}
		if !strings.HasPrefix(subcommands[index], "--") && validCommand {
			if index != 0 {
				subcommandsSplit = append(subcommandsSplit, &subcommand{command: subcommands[prevSplitIndex], flags: subcommands[prevSplitIndex+1 : index]})
			}
			prevSplitIndex = index
		}
	}
	subcommandsSplit = append(subcommandsSplit, &subcommand{command: subcommands[prevSplitIndex], flags: subcommands[prevSplitIndex+1:]})

	return subcommandsSplit
}

func accountUsage() {
	fmt.Println("Usage for accounts [COMMAND] [FLAGS], multiple commands can be chained together.")
	fmt.Printf("COMMAND: %s\n", accountwallet.CreateAccountCommand)
	parseCreateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.DestroyAccountCommand)
	parseDestroyAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.AllotAccountCommand)
	parseAllotAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n No flags available.", accountwallet.AllotAccountCommand)
}

func parseCreateAccountFlags(subcommands []string) *accountwallet.CreateAccountParams {
	flagSet := flag.NewFlagSet("script flag set", flag.ExitOnError)

	alias := flagSet.String("alias", "", "Alias of the account to be created")
	amount := flagSet.Int("amount", 100, "Amount of foucet tokens to be used for the accountcreation")

	if subcommands == nil {
		flagSet.Usage()

		return nil
	}

	log.Infof("Parsing create account flags, subcommands: %v", subcommands)

	err := flagSet.Parse(subcommands)
	if err != nil {
		log.Errorf("Cannot parse first `script` parameter")

		return nil
	}

	createAccountParams := &accountwallet.CreateAccountParams{
		Alias:  *alias,
		Amount: uint64(*amount),
	}
	return createAccountParams
}

func parseDestroyAccountFlags(subcommands []string) *accountwallet.DestroyAccountParams {
	flagSet := flag.NewFlagSet("script flag set", flag.ExitOnError)

	alias := flagSet.String("alias", "", "Alias of the account to be destroyed")

	if subcommands == nil {
		flagSet.Usage()

		return nil
	}

	log.Infof("Parsing destroy account flags, subcommands: %v", subcommands)
	err := flagSet.Parse(subcommands)
	if err != nil {
		log.Errorf("Cannot parse first `script` parameter")

		return nil
	}
	createAccountParams := &accountwallet.DestroyAccountParams{
		AccountAlias: *alias,
	}
	return createAccountParams
}

func parseAllotAccountFlags(subcommands []string) *accountwallet.AllotAccountParams {
	flagSet := flag.NewFlagSet("script flag set", flag.ExitOnError)

	to := flagSet.String("to", "", "Alias of the account to allot mana")
	amount := flagSet.Int("amount", 100, "Amount of mana to allot")
	from := flagSet.String("from", "", "Alias of the account we allot from, if not specified, we allot from the faucet account")

	if subcommands == nil {
		flagSet.Usage()

		return nil
	}

	log.Infof("Parsing allot account flags, subcommands: %v", subcommands)
	err := flagSet.Parse(subcommands)
	if err != nil {
		log.Errorf("Cannot parse first `script` parameter")

		return nil
	}

	createAccountParams := &accountwallet.AllotAccountParams{
		To:     *to,
		Amount: uint64(*amount),
	}

	if *from != "" {
		createAccountParams.From = *from
	}

	return createAccountParams
}

// func parseCommitmentsSpamFlags() {
// 	commitmentType := optionFlagSet.String("type", commitmentsSpamParams.CommitmentType, "Type of commitment spam. Possible values: 'latest' - valid commitment spam, 'random' - completely new, invalid cahin, 'fork' - forked chain, combine with 'forkAfter' parameter.")
// 	rate := optionFlagSet.Int("rate", commitmentsSpamParams.Rate, "Commitment spam rate")
// 	duration := optionFlagSet.Duration("duration", commitmentsSpamParams.Duration, "Duration of the spam. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
// 	timeUnit := optionFlagSet.Duration("tu", commitmentsSpamParams.TimeUnit, "Time unit for the spamming rate. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
// 	networkAlias := optionFlagSet.String("network", commitmentsSpamParams.NetworkAlias, "Network alias for the test. Check your keys-config.json file for possible values.")
// 	identityAlias := optionFlagSet.String("spammerAlias", commitmentsSpamParams.SpammerAlias, "Identity alias for the node identity and its private keys that will be used to spam. Check your keys-config.json file for possible values.")
// 	validAlias := optionFlagSet.String("validAlias", commitmentsSpamParams.ValidAlias, "Identity alias for the honest node and its private keys, will be used to request valid commitment and block data. Check your keys-config.json file for possible values.")
// 	forkAfter := optionFlagSet.Int("forkAfter", commitmentsSpamParams.Rate, "Indicates how many slots after spammer startup should fork be placed in the created commitment chain. Works only for 'fork' commitment spam type.")

// 	parseOptionFlagSet(optionFlagSet)

// 	commitmentsSpamParams.CommitmentType = *commitmentType
// 	commitmentsSpamParams.Rate = *rate
// 	commitmentsSpamParams.Duration = *duration
// 	commitmentsSpamParams.TimeUnit = *timeUnit
// 	commitmentsSpamParams.NetworkAlias = *networkAlias
// 	commitmentsSpamParams.SpammerAlias = *identityAlias
// 	commitmentsSpamParams.ValidAlias = *validAlias
// 	commitmentsSpamParams.ForkAfter = *forkAfter
// }

func parseCommaSepString(urls string) []string {
	split := strings.Split(urls, ",")

	return split
}

func parseCommaSepInt(nums string) []int {
	split := strings.Split(nums, ",")
	parsed := make([]int, len(split))
	for i, num := range split {
		parsed[i], _ = strconv.Atoi(num)
	}

	return parsed
}

func parseDurations(durations string) []time.Duration {
	split := strings.Split(durations, ",")
	parsed := make([]time.Duration, len(split))
	for i, dur := range split {
		parsed[i], _ = time.ParseDuration(dur)
	}

	return parsed
}
