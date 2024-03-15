package toolset

import (
	"encoding/hex"
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/app/configuration"
	"github.com/iotaledger/hive.go/crypto"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

type walletInfo struct {
	BIP39          string `json:"mnemonic,omitempty"`
	BIP32          string `json:"path,omitempty"`
	PrivateKey     string `json:"privateKey,omitempty"`
	PublicKey      string `json:"publicKey"`
	Ed25519Address string `json:"ed25519"`
	Bech32Address  string `json:"bech32"`
}

func printKeyManagerInfo(keyManager *wallet.KeyManager, hrp iotago.NetworkPrefix, outputJSON bool) error {
	addr := keyManager.Address(iotago.AddressEd25519)
	privKey, pubKey := keyManager.KeyPair()

	w := walletInfo{
		PublicKey:      hex.EncodeToString(pubKey),
		PrivateKey:     hex.EncodeToString(privKey),
		Ed25519Address: addr.String(),
		Bech32Address:  addr.Bech32(hrp),
		BIP39:          keyManager.Mnemonic().String(),
		BIP32:          keyManager.Path().String(),
	}

	return printWalletInfo(w, outputJSON)
}

func printWalletInfo(info walletInfo, outputJSON bool) error {
	if outputJSON {
		return printJSON(info)
	}

	if len(info.BIP39) > 0 {
		fmt.Println("Your seed BIP39 mnemonic: ", info.BIP39)
		fmt.Println()
		fmt.Println("Your BIP32 path:          ", info.BIP32)
	}

	if info.PrivateKey != "" {
		fmt.Println("Your ed25519 private key: ", info.PrivateKey)
	}

	fmt.Println("Your ed25519 public key:  ", info.PublicKey)
	fmt.Println("Your ed25519 address:     ", info.Ed25519Address)
	fmt.Println("Your bech32 address:      ", info.Bech32Address)

	return nil
}

func generateEd25519Key(args []string) error {
	fs := configuration.NewUnsortedFlagSet("", flag.ContinueOnError)
	hrpFlag := fs.String(FlagToolHRP, string(iotago.PrefixTestnet), "the HRP which should be used for the Bech32 address")
	bip32Path := fs.String(FlagToolBIP32Path, "m/44'/4218'/0'/0'/0'", "the BIP32 path that should be used to derive keys from seed")
	mnemonicFlag := fs.String(FlagToolMnemonic, "", "the BIP-39 mnemonic sentence that should be used to derive the seed from (optional)")
	outputJSONFlag := fs.Bool(FlagToolOutputJSON, false, FlagToolDescriptionOutputJSON)

	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", ToolEd25519Key)
		fs.PrintDefaults()
		println(fmt.Sprintf("\nexample: %s --%s %s",
			ToolEd25519Key,
			FlagToolHRP,
			string(iotago.PrefixTestnet)))
	}

	if err := parseFlagSet(fs, args); err != nil {
		return err
	}

	if len(*hrpFlag) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolHRP)
	}

	if len(*bip32Path) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolBIP32Path)
	}

	var err error
	var keyManager *wallet.KeyManager
	if len(*mnemonicFlag) == 0 {
		keyManager, err = wallet.NewKeyManagerFromRandom(*bip32Path)
		if err != nil {
			return err
		}
	} else {
		keyManager, err = wallet.NewKeyManagerFromMnemonic(*mnemonicFlag, *bip32Path)
		if err != nil {
			return err
		}
	}

	return printKeyManagerInfo(keyManager, iotago.NetworkPrefix(*hrpFlag), *outputJSONFlag)
}

func generateEd25519Address(args []string) error {
	fs := configuration.NewUnsortedFlagSet("", flag.ContinueOnError)
	hrpFlag := fs.String(FlagToolHRP, string(iotago.PrefixTestnet), "the HRP which should be used for the Bech32 address")
	publicKeyFlag := fs.String(FlagToolPublicKey, "", "an ed25519 public key")
	outputJSONFlag := fs.Bool(FlagToolOutputJSON, false, FlagToolDescriptionOutputJSON)

	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", ToolEd25519Addr)
		fs.PrintDefaults()
		println(fmt.Sprintf("\nexample: %s --%s %s --%s %s",
			ToolEd25519Addr,
			FlagToolHRP,
			string(iotago.PrefixTestnet),
			FlagToolPublicKey,
			"[PUB_KEY]",
		))
	}

	if err := parseFlagSet(fs, args); err != nil {
		return err
	}

	if len(*hrpFlag) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolHRP)
	}

	if len(*publicKeyFlag) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolPublicKey)
	}

	// parse pubkey
	pubKey, err := crypto.ParseEd25519PublicKeyFromString(*publicKeyFlag)
	if err != nil {
		return ierrors.Wrapf(err, "can't decode '%s'", FlagToolPublicKey)
	}

	addr := iotago.Ed25519AddressFromPubKey(pubKey)

	hrp := iotago.NetworkPrefix(*hrpFlag)

	w := walletInfo{
		PublicKey:      hex.EncodeToString(pubKey),
		Ed25519Address: addr.String(),
		Bech32Address:  addr.Bech32(hrp),
	}

	return printWalletInfo(w, *outputJSONFlag)
}
