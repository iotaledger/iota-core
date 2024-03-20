package toolset

import (
	"fmt"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/app/configuration"
	hivep2p "github.com/iotaledger/hive.go/crypto/p2p"
	"github.com/iotaledger/hive.go/crypto/pem"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/components/p2p"
)

func extractP2PIdentity(args []string) error {
	fs := configuration.NewUnsortedFlagSet("", flag.ContinueOnError)
	databasePathFlag := fs.String(FlagToolDatabasePath, DefaultValueP2PDatabasePath, "the path to the p2p database folder")
	outputJSONFlag := fs.Bool(FlagToolOutputJSON, false, FlagToolDescriptionOutputJSON)

	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", ToolP2PExtractIdentity)
		fs.PrintDefaults()
		println(fmt.Sprintf("\nexample: %s --%s %s",
			ToolP2PExtractIdentity,
			FlagToolDatabasePath,
			DefaultValueP2PDatabasePath))
	}

	if err := parseFlagSet(fs, args); err != nil {
		return err
	}

	if len(*databasePathFlag) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolDatabasePath)
	}

	databasePath := *databasePathFlag
	privKeyFilePath := filepath.Join(databasePath, p2p.IdentityPrivateKeyFileName)

	_, err := os.Stat(privKeyFilePath)
	switch {
	case os.IsNotExist(err):
		// private key does not exist
		return ierrors.Errorf("private key file (%s) does not exist", privKeyFilePath)

	case err == nil || os.IsExist(err):
		// private key file exists

	default:
		return ierrors.Wrapf(err, "unable to check private key file (%s)", privKeyFilePath)
	}

	privKey, err := pem.ReadEd25519PrivateKeyFromPEMFile(privKeyFilePath)
	if err != nil {
		return ierrors.Wrap(err, "reading private key file for peer identity failed")
	}

	libp2pPrivKey, err := hivep2p.Ed25519PrivateKeyToLibp2pPrivateKey(privKey)
	if err != nil {
		return err
	}

	return printP2PIdentity(libp2pPrivKey, libp2pPrivKey.GetPublic(), *outputJSONFlag)
}
