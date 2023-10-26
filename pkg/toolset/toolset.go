package toolset

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/app/configuration"
	"github.com/iotaledger/hive.go/ierrors"
)

const (
	FlagToolDatabasePath = "databasePath"

	FlagToolOutputPath = "outputPath"

	FlagToolPrivateKey = "privateKey"
	FlagToolPublicKey  = "publicKey"

	FlagToolHRP       = "hrp"
	FlagToolBIP32Path = "bip32Path"
	FlagToolMnemonic  = "mnemonic"
	FlagToolSalt      = "salt"

	FlagToolNodeURL = "nodeURL"

	FlagToolOutputJSON            = "json"
	FlagToolDescriptionOutputJSON = "format output as JSON"
)

const (
	ToolP2PIdentityGen     = "p2pidentity-gen"
	ToolP2PExtractIdentity = "p2pidentity-extract"
	ToolEd25519Key         = "ed25519-key"
	ToolEd25519Addr        = "ed25519-addr"
	ToolJWTApi             = "jwt-api"
	ToolNodeInfo           = "node-info"
)

const (
	DefaultValueAPIJWTTokenSalt = "IOTA"
	DefaultValueP2PDatabasePath = "testnet/p2pstore"
)

// ShouldHandleTools checks if tools were requested.
func ShouldHandleTools() bool {
	args := os.Args[1:]

	for _, arg := range args {
		if strings.ToLower(arg) == "tool" || strings.ToLower(arg) == "tools" {
			return true
		}
	}

	return false
}

// HandleTools handles available tools.
func HandleTools() {

	args := os.Args[1:]
	if len(args) == 1 {
		listTools()
		os.Exit(1)
	}

	tools := map[string]func([]string) error{
		ToolP2PIdentityGen:     generateP2PIdentity,
		ToolP2PExtractIdentity: extractP2PIdentity,
		ToolEd25519Key:         generateEd25519Key,
		ToolEd25519Addr:        generateEd25519Address,
		ToolJWTApi:             generateJWTApiToken,
		ToolNodeInfo:           nodeInfo,
	}

	tool, exists := tools[strings.ToLower(args[1])]
	if !exists {
		fmt.Print("tool not found.\n\n")
		listTools()
		os.Exit(1)
	}

	if err := tool(args[2:]); err != nil {
		if ierrors.Is(err, flag.ErrHelp) {
			// help text was requested
			os.Exit(0)
		}

		fmt.Printf("\nerror: %s\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func listTools() {
	fmt.Printf("%-20s generates a p2p identity private key file\n", fmt.Sprintf("%s:", ToolP2PIdentityGen))
	fmt.Printf("%-20s extracts the p2p identity from the private key file\n", fmt.Sprintf("%s:", ToolP2PExtractIdentity))
	fmt.Printf("%-20s generates an ed25519 key pair\n", fmt.Sprintf("%s:", ToolEd25519Key))
	fmt.Printf("%-20s generates an ed25519 address from a public key\n", fmt.Sprintf("%s:", ToolEd25519Addr))
	fmt.Printf("%-20s generates a JWT token for REST-API access\n", fmt.Sprintf("%s:", ToolJWTApi))
	fmt.Printf("%-20s queries the info endpoint of a node\n", fmt.Sprintf("%s:", ToolNodeInfo))
}

func yesOrNo(value bool) string {
	if value {
		return "YES"
	}

	return "NO"
}

func parseFlagSet(fs *flag.FlagSet, args []string) error {

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check if all parameters were parsed
	if fs.NArg() != 0 {
		return ierrors.New("too much arguments")
	}

	return nil
}

func printJSON(obj interface{}) error {
	output, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(output))

	return nil
}

//nolint:unused // we will need it at a later point in time
func loadConfigFile(filePath string, parameters map[string]any) error {
	config := configuration.New()
	flagset := configuration.NewUnsortedFlagSet("", flag.ContinueOnError)

	for namespace, pointerToStruct := range parameters {
		config.BindParameters(flagset, namespace, pointerToStruct)
	}

	if err := config.LoadFile(filePath); err != nil {
		return fmt.Errorf("loading config file failed: %w", err)
	}

	config.UpdateBoundParameters()

	return nil
}
