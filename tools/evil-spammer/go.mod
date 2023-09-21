module github.com/iotaledger/iota-core/tools/evil-spammer

go 1.21

replace github.com/iotaledger/iota-core => ../../

replace github.com/iotaledger/iota-core/tools/genesis-snapshot => ../genesis-snapshot/

require (
	github.com/AlecAivazis/survey/v2 v2.3.7
	github.com/iotaledger/hive.go/app v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/hive.go/crypto v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/hive.go/ds v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/hive.go/ierrors v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/hive.go/lo v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/hive.go/logger v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/hive.go/runtime v0.0.0-20230912172434-dc477e1f5140
	github.com/iotaledger/iota-core v0.0.0-00010101000000-000000000000
	github.com/iotaledger/iota-core/tools/genesis-snapshot v0.0.0-00010101000000-000000000000
	github.com/iotaledger/iota.go/v4 v4.0.0-20230921110244-f4f25eb27e05
	github.com/mr-tron/base58 v1.2.0
	go.uber.org/atomic v1.11.0
)

require (
	filippo.io/edwards25519 v1.0.0 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/eclipse/paho.mqtt.golang v1.4.3 // indirect
	github.com/ethereum/go-ethereum v1.13.1 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/holiman/uint256 v1.2.3 // indirect
	github.com/iancoleman/orderedmap v0.3.0 // indirect
	github.com/iotaledger/grocksdb v1.7.5-0.20230220105546-5162e18885c7 // indirect
	github.com/iotaledger/hive.go/ads v0.0.0-20230906114834-b50190b9f9c2 // indirect
	github.com/iotaledger/hive.go/constraints v0.0.0-20230912172434-dc477e1f5140 // indirect
	github.com/iotaledger/hive.go/core v1.0.0-rc.3.0.20230912172434-dc477e1f5140 // indirect
	github.com/iotaledger/hive.go/kvstore v0.0.0-20230906114834-b50190b9f9c2 // indirect
	github.com/iotaledger/hive.go/serializer/v2 v2.0.0-rc.1.0.20230912172434-dc477e1f5140 // indirect
	github.com/iotaledger/hive.go/stringify v0.0.0-20230912172434-dc477e1f5140 // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/knadh/koanf v1.5.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-libp2p v0.30.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.11.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multicodec v0.9.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/otiai10/copy v1.12.0 // indirect
	github.com/pasztorpisti/qs v0.0.0-20171216220353-8d6c33ee906c // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/petermattis/goid v0.0.0-20230904192822-1876fd5063bc // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/pokt-network/smt v0.6.1 // indirect
	github.com/sasha-s/go-deadlock v0.3.1 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/wollac/iota-crypto-demo v0.0.0-20221117162917-b10619eccb98 // indirect
	github.com/zyedidia/generic v1.2.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/crypto v0.13.0 // indirect
	golang.org/x/exp v0.0.0-20230817173708-d852ddb80c63 // indirect
	golang.org/x/net v0.15.0 // indirect
	golang.org/x/sync v0.3.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/term v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)
