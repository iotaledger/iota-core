//nolint:forcetypeassert
package depositcalculator

import (
	"math/big"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Options struct {
	// UnlockConditions
	Address                     iotago.Address
	StorageDepositReturnAddress iotago.Address
	HasTimelock                 bool
	ExpirationAddress           iotago.Address
	StateControllerAddress      iotago.Address
	GovernorAddress             iotago.Address

	// Features
	SenderAddress               iotago.Address
	IssuerAddress               iotago.Address
	MetadataSerializedSize      int
	StateMetadataSerializedSize int
	TagLength                   int
	HasNativeToken              bool
	BlockIssuerKeys             int
	StakedAmount                iotago.BaseToken

	// Immutable Features
	ImmutableIssuerAddress          iotago.Address
	ImmutableMetadataSerializedSize int
}

// WithAddress sets the address for the address unlock condition.
func WithAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.Address = address
	}
}

// WithStorageDepositReturnAddress adds a storage deposit return unlock condition and sets the address.
func WithStorageDepositReturnAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.StorageDepositReturnAddress = address
	}
}

// WithHasTimelock adds a timelock unlock condition.
func WithHasTimelock() options.Option[Options] {
	return func(opts *Options) {
		opts.HasTimelock = true
	}
}

// WithExpirationAddress adds a expiration unlock condition and sets the address.
func WithExpirationAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.ExpirationAddress = address
	}
}

// WithStateControllerAddress sets the address for the state controller address unlock condition.
func WithStateControllerAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.StateControllerAddress = address
	}
}

// WithGovernorAddress sets the address for the governor address unlock condition.
func WithGovernorAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.GovernorAddress = address
	}
}

// WithSenderAddress adds a sender feature and sets the address.
func WithSenderAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.SenderAddress = address
	}
}

// WithIssuerAddress adds a issuer feature and sets the address.
func WithIssuerAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.IssuerAddress = address
	}
}

// WithMetadataSerializedSize adds a metadata feature and adds an entry with an empty key and a dummy value
// of the given length minus the length of the map length prefix, key length prefix and value length prefix.
func WithMetadataSerializedSize(size int) options.Option[Options] {
	return func(opts *Options) {
		opts.MetadataSerializedSize = size
	}
}

// WithStateMetadataSerializedSize adds a state metadata feature and adds an entry with an empty key and a dummy value
// of the given length minus the length of the map length prefix, key length prefix and value length prefix.
func WithStateMetadataSerializedSize(size int) options.Option[Options] {
	return func(opts *Options) {
		opts.StateMetadataSerializedSize = size
	}
}

// WithTagLength adds a tag feature and sets a dummy tag with the given length.
func WithTagLength(length int) options.Option[Options] {
	return func(opts *Options) {
		opts.TagLength = length
	}
}

// WithHasNativeToken adds a native token feature.
func WithHasNativeToken() options.Option[Options] {
	return func(opts *Options) {
		opts.HasNativeToken = true
	}
}

// WithBlockIssuerKeys adds a block issuer feature and adds the given amount of dummy keys.
func WithBlockIssuerKeys(keys int) options.Option[Options] {
	return func(opts *Options) {
		opts.BlockIssuerKeys = keys
	}
}

// WithStakedAmount adds a staking feature and sets the staked amount.
func WithStakedAmount(amount iotago.BaseToken) options.Option[Options] {
	return func(opts *Options) {
		opts.StakedAmount = amount
	}
}

// WithImmutableIssuerAddress adds an immutable issuer feature and sets the address.
func WithImmutableIssuerAddress(address iotago.Address) options.Option[Options] {
	return func(opts *Options) {
		opts.ImmutableIssuerAddress = address
	}
}

// WithImmutableMetadataSerializedSize adds an immutable metadata feature and adds an entry with an empty key and a dummy value
// of the given length minus the length of the map length prefix, key length prefix and value length prefix.
func WithImmutableMetadataSerializedSize(size int) options.Option[Options] {
	return func(opts *Options) {
		opts.ImmutableMetadataSerializedSize = size
	}
}

func getUnlockConditions[T iotago.UnlockCondition](outputType iotago.OutputType, opts *Options) iotago.UnlockConditions[T] {
	unlockConditions := make(iotago.UnlockConditions[T], 0)

	// Mandatory address unlocks
	switch outputType {
	case iotago.OutputBasic, iotago.OutputAccount, iotago.OutputNFT, iotago.OutputDelegation:
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.AddressUnlockCondition{Address: opts.Address}).(T))

	case iotago.OutputAnchor:
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.StateControllerAddressUnlockCondition{Address: opts.StateControllerAddress}).(T))
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.GovernorAddressUnlockCondition{Address: opts.GovernorAddress}).(T))

	case iotago.OutputFoundry:
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.ImmutableAccountUnlockCondition{Address: &iotago.AccountAddress{}}).(T))
	}

	if opts.StorageDepositReturnAddress != nil {
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.StorageDepositReturnUnlockCondition{ReturnAddress: opts.StorageDepositReturnAddress}).(T))
	}

	if opts.HasTimelock {
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.TimelockUnlockCondition{}).(T))
	}

	if opts.ExpirationAddress != nil {
		unlockConditions = append(unlockConditions, iotago.UnlockCondition(&iotago.ExpirationUnlockCondition{ReturnAddress: opts.ExpirationAddress}).(T))
	}

	return unlockConditions
}

func getFeatures[T iotago.Feature](opts *Options) iotago.Features[T] {
	features := iotago.Features[iotago.Feature]{}

	if opts.SenderAddress != nil {
		features = append(features, &iotago.SenderFeature{Address: opts.SenderAddress})
	}

	if opts.IssuerAddress != nil {
		features = append(features, &iotago.IssuerFeature{Address: opts.IssuerAddress})
	}

	if opts.MetadataSerializedSize > 0 {
		features = append(features, &iotago.MetadataFeature{
			Entries: iotago.MetadataFeatureEntries{
				// configured length - map length prefix - key length prefix - value length prefix
				"": make([]byte, opts.MetadataSerializedSize-int(serix.LengthPrefixTypeAsByte)+int(serix.LengthPrefixTypeAsByte)+int(serix.LengthPrefixTypeAsUint16)),
			},
		})
	}

	if opts.StateMetadataSerializedSize > 0 {
		features = append(features, &iotago.StateMetadataFeature{
			Entries: iotago.StateMetadataFeatureEntries{
				// configured length - map length prefix - key length prefix - value length prefix
				"": make([]byte, opts.StateMetadataSerializedSize-int(serix.LengthPrefixTypeAsByte)+int(serix.LengthPrefixTypeAsByte)+int(serix.LengthPrefixTypeAsUint16)),
			},
		})
	}

	if opts.TagLength > 0 {
		features = append(features, &iotago.TagFeature{Tag: make([]byte, opts.TagLength)})
	}

	if opts.HasNativeToken {
		features = append(features, &iotago.NativeTokenFeature{Amount: big.NewInt(0)})
	}

	if opts.BlockIssuerKeys > 0 {
		blockIssuerKeys := make([]iotago.BlockIssuerKey, 0, opts.BlockIssuerKeys)
		for range opts.BlockIssuerKeys {
			blockIssuerKeys = append(blockIssuerKeys, &iotago.Ed25519PublicKeyHashBlockIssuerKey{})
		}

		features = append(features, &iotago.BlockIssuerFeature{BlockIssuerKeys: blockIssuerKeys})
	}

	if opts.StakedAmount > 0 {
		features = append(features, &iotago.StakingFeature{StakedAmount: opts.StakedAmount})
	}

	feats := iotago.Features[T]{}
	for _, feat := range features {
		//nolint:forcetypeassert
		feats = append(feats, feat.(T))
	}

	return feats
}

func getImmutableFeatures[T iotago.Feature](opts *Options) iotago.Features[T] {
	features := iotago.Features[iotago.Feature]{}

	if opts.ImmutableIssuerAddress != nil {
		features = append(features, &iotago.IssuerFeature{Address: opts.ImmutableIssuerAddress})
	}

	if opts.ImmutableMetadataSerializedSize > 0 {
		features = append(features, &iotago.MetadataFeature{
			Entries: iotago.MetadataFeatureEntries{
				// configured length - map length prefix - key length prefix - value length prefix
				"": make([]byte, opts.ImmutableMetadataSerializedSize-int(serix.LengthPrefixTypeAsByte)+int(serix.LengthPrefixTypeAsByte)+int(serix.LengthPrefixTypeAsUint16)),
			},
		})
	}

	feats := iotago.Features[T]{}
	for _, feat := range features {
		//nolint:forcetypeassert
		feats = append(feats, feat.(T))
	}

	return feats
}

func MinDeposit(protocolParams iotago.ProtocolParameters, outputType iotago.OutputType, opts ...options.Option[Options]) (iotago.BaseToken, error) {
	depositCalculationOptions := options.Apply(&Options{}, opts)

	var dummyOutput iotago.Output
	switch outputType {
	case iotago.OutputBasic:
		dummyOutput = &iotago.BasicOutput{
			Amount:           0,
			Mana:             0,
			UnlockConditions: getUnlockConditions[iotago.BasicOutputUnlockCondition](outputType, depositCalculationOptions),
			Features:         getFeatures[iotago.BasicOutputFeature](depositCalculationOptions),
		}

	case iotago.OutputAccount:
		dummyOutput = &iotago.AccountOutput{
			Amount:            0,
			Mana:              0,
			AccountID:         iotago.AccountID{},
			FoundryCounter:    0,
			UnlockConditions:  getUnlockConditions[iotago.AccountOutputUnlockCondition](outputType, depositCalculationOptions),
			Features:          getFeatures[iotago.AccountOutputFeature](depositCalculationOptions),
			ImmutableFeatures: getImmutableFeatures[iotago.AccountOutputImmFeature](depositCalculationOptions),
		}

	case iotago.OutputAnchor:
		dummyOutput = &iotago.AnchorOutput{
			Amount:            0,
			Mana:              0,
			AnchorID:          iotago.AnchorID{},
			StateIndex:        0,
			UnlockConditions:  getUnlockConditions[iotago.AnchorOutputUnlockCondition](outputType, depositCalculationOptions),
			Features:          getFeatures[iotago.AnchorOutputFeature](depositCalculationOptions),
			ImmutableFeatures: getImmutableFeatures[iotago.AnchorOutputImmFeature](depositCalculationOptions),
		}

	case iotago.OutputFoundry:
		dummyOutput = &iotago.FoundryOutput{
			Amount:       0,
			SerialNumber: 0,
			TokenScheme: &iotago.SimpleTokenScheme{
				MintedTokens:  big.NewInt(0),
				MeltedTokens:  big.NewInt(0),
				MaximumSupply: big.NewInt(0),
			},
			UnlockConditions:  getUnlockConditions[iotago.FoundryOutputUnlockCondition](outputType, depositCalculationOptions),
			Features:          getFeatures[iotago.FoundryOutputFeature](depositCalculationOptions),
			ImmutableFeatures: getImmutableFeatures[iotago.FoundryOutputImmFeature](depositCalculationOptions),
		}

	case iotago.OutputNFT:
		dummyOutput = &iotago.NFTOutput{
			Amount:            0,
			Mana:              0,
			NFTID:             iotago.NFTID{},
			UnlockConditions:  getUnlockConditions[iotago.NFTOutputUnlockCondition](outputType, depositCalculationOptions),
			Features:          getFeatures[iotago.NFTOutputFeature](depositCalculationOptions),
			ImmutableFeatures: getImmutableFeatures[iotago.NFTOutputImmFeature](depositCalculationOptions),
		}

	case iotago.OutputDelegation:
		dummyOutput = &iotago.DelegationOutput{
			Amount:           0,
			DelegatedAmount:  0,
			DelegationID:     iotago.DelegationID{},
			ValidatorAddress: &iotago.AccountAddress{},
			StartEpoch:       0,
			EndEpoch:         0,
			UnlockConditions: getUnlockConditions[iotago.DelegationOutputUnlockCondition](outputType, depositCalculationOptions),
		}
	}

	storageScoreStructure := iotago.NewStorageScoreStructure(protocolParams.StorageScoreParameters())

	// calculate the minimum storage deposit
	minDeposit, err := storageScoreStructure.MinDeposit(dummyOutput)
	if err != nil {
		return 0, err
	}

	// we need to have at least the staked amount in the deposit
	if depositCalculationOptions.StakedAmount > minDeposit {
		minDeposit = depositCalculationOptions.StakedAmount
	}

	return minDeposit, nil
}
