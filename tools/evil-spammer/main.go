package main

import (
	"flag"
	"fmt"

	"github.com/iotaledger/iota-core/tools/evil-spammer/accountwallet"
	"github.com/iotaledger/iota-core/tools/evil-spammer/interactive"
	"github.com/iotaledger/iota-core/tools/evil-spammer/logger"
	"github.com/iotaledger/iota-core/tools/evil-spammer/programs"
)

const (
	accountWalletFilename = "wallet.dat"
)

var (
	log           = logger.New("main")
	optionFlagSet = flag.NewFlagSet("script flag set", flag.ExitOnError)
)

func main() {
	help := parseFlags()

	if help {
		fmt.Println("Usage of the Evil Spammer tool, provide the first argument for the selected mode:\n" +
			"'interactive' - enters the interactive mode.\n" +
			"'basic' - can be parametrized with additional flags to run one time spammer. Run 'evil-wallet basic -h' for the list of possible flags.\n" +
			"'quick' - runs simple stress test: tx spam -> blk spam -> ds spam. Run 'evil-wallet quick -h' for the list of possible flags.\n" +
			"'commitments' - runs spammer for commitments. Run 'evil-wallet commitments -h' for the list of possible flags.")

		return
	}
	// run selected test scenario
	switch Script {
	case "interactive":
		interactive.Run()
	case "basic":
		programs.CustomSpam(&customSpamParams)
	case "accounts":
		// TODO init wallet for the first time if no file was found
		// load wallet
		accWallet, err := accountwallet.Run(accountWalletFilename)
		if err != nil {
			log.Warn(err)
			return
		}
		// save wallet
		defer func(w *accountwallet.AccountWallet, filename string) {
			err = accountwallet.SaveState(w, filename)
			if err != nil {
				log.Errorf("Error while saving wallet state: %v", err)
			}
		}(accWallet, "wallet.dat")
		accountsSubcommands(accWallet, accountsSubcommandsFlags)
	case "quick":
		programs.QuickTest(&quickTestParams)
	// case SpammerTypeCommitments:
	// 	CommitmentsSpam(&commitmentsSpamParams)
	default:
		log.Warnf("Unknown parameter for script, possible values: basic, quick, commitments")
	}
}

func accountsSubcommands(wallet *accountwallet.AccountWallet, subcommands []*subcommand) {
	for _, sub := range subcommands {
		accountsSubcommand(wallet, sub)
	}
}

func accountsSubcommand(wallet *accountwallet.AccountWallet, sub *subcommand) {
	switch sub.command {
	case accountwallet.CreateAccountCommand:
		params := parseCreateAccountFlags(sub.flags)
		log.Infof("Run subcommand: %s, with parametetr set: %v", accountwallet.CreateAccountCommand, params)
		err := wallet.CreateAccount(params)
		if err != nil {
			log.Errorf("Error creating account: %v", err)

			return
		}
	case accountwallet.DestroyAccountCommand:
		params := parseDestroyAccountFlags(sub.flags)
		log.Infof("Run subcommand: %s, with parametetr set: %v", accountwallet.DestroyAccountCommand, params)

		err := wallet.DestroyAccount(params)
		if err != nil {
			log.Errorf("Error destroying account: %v", err)

			return
		}
	case accountwallet.ListAccountsCommand:
		err := wallet.ListAccount()
		if err != nil {
			log.Errorf("Error listing accounts: %v", err)

			return
		}
	case accountwallet.AllotAccountCommand:
		params := parseAllotAccountFlags(sub.flags)
		log.Infof("Run subcommand: %s, with parametetr set: %v", accountwallet.AllotAccountCommand, params)

		err := wallet.AllotToAccount(params)
		if err != nil {
			log.Errorf("Error allotting account: %v", err)

			return
		}
	}
}
