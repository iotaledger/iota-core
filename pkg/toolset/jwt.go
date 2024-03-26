package toolset

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/app/configuration"
	hivep2p "github.com/iotaledger/hive.go/crypto/p2p"
	"github.com/iotaledger/hive.go/crypto/pem"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/components/p2p"
	"github.com/iotaledger/iota-core/pkg/jwt"
)

func generateJWTApiToken(args []string) error {
	fs := configuration.NewUnsortedFlagSet("", flag.ContinueOnError)
	databasePathFlag := fs.String(FlagToolDatabasePath, DefaultValueP2PDatabasePath, "the path to the p2p database folder")
	apiJWTSaltFlag := fs.String(FlagToolSalt, DefaultValueAPIJWTTokenSalt, "salt used inside the JWT tokens for the REST API")
	outputJSONFlag := fs.Bool(FlagToolOutputJSON, false, FlagToolDescriptionOutputJSON)

	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", ToolJWTApi)
		fs.PrintDefaults()
		println(fmt.Sprintf("\nexample: %s --%s %s --%s %s",
			ToolJWTApi,
			FlagToolDatabasePath,
			DefaultValueP2PDatabasePath,
			FlagToolSalt,
			DefaultValueAPIJWTTokenSalt))
	}

	if err := parseFlagSet(fs, args); err != nil {
		return err
	}

	if len(*databasePathFlag) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolDatabasePath)
	}
	if len(*apiJWTSaltFlag) == 0 {
		return ierrors.Errorf("'%s' not specified", FlagToolSalt)
	}

	databasePath := *databasePathFlag
	privKeyFilePath := filepath.Join(databasePath, p2p.IdentityPrivateKeyFileName)

	salt := *apiJWTSaltFlag

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
		return ierrors.Wrap(err, "reading private key file for peer identity failed")
	}

	peerID, err := peer.IDFromPublicKey(libp2pPrivKey.GetPublic())
	if err != nil {
		return ierrors.Wrap(err, "unable to get peer identity from public key")
	}

	// API tokens do not expire.
	jwtAuth, err := jwt.NewAuth(salt,
		0,
		peerID.String(),
		libp2pPrivKey,
	)
	if err != nil {
		return ierrors.Wrap(err, "JWT auth initialization failed")
	}

	jwtToken, err := jwtAuth.IssueJWT()
	if err != nil {
		return ierrors.Wrap(err, "issuing JWT token failed")
	}

	if *outputJSONFlag {
		result := struct {
			JWT string `json:"jwt"`
		}{
			JWT: jwtToken,
		}

		return printJSON(result)
	}

	fmt.Println("Your API JWT token: ", jwtToken)

	return nil
}
