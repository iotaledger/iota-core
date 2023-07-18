package api

import (
	"context"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

type anyAPI struct {
	serixAPI *serix.API

	protocolParameters iotago.ProtocolParameters
	timeProvider       *iotago.TimeProvider
	manaDecayProvider  *iotago.ManaDecayProvider
}

func NewAnyAPI(protocolParameters iotago.ProtocolParameters) iotago.API {
	v3API := iotago.V3API(iotago.NewV3ProtocolParameters())

	return &anyAPI{
		serixAPI:           v3API.Underlying(), // this is needed to get all the default serix configuration
		protocolParameters: protocolParameters,
		timeProvider:       protocolParameters.TimeProvider(),
		manaDecayProvider:  protocolParameters.ManaDecayProvider(),
	}
}

func (v *anyAPI) JSONEncode(obj any, opts ...serix.Option) ([]byte, error) {
	return v.serixAPI.JSONEncode(context.TODO(), obj, opts...)
}

func (v *anyAPI) JSONDecode(jsonData []byte, obj any, opts ...serix.Option) error {
	return v.serixAPI.JSONDecode(context.TODO(), jsonData, obj, opts...)
}

func (v *anyAPI) Underlying() *serix.API {
	return v.serixAPI
}

func (v *anyAPI) Version() iotago.Version {
	return v.protocolParameters.Version()
}

func (v *anyAPI) ProtocolParameters() iotago.ProtocolParameters {
	return v.protocolParameters
}

func (v *anyAPI) TimeProvider() *iotago.TimeProvider {
	return v.timeProvider
}

func (v *anyAPI) ManaDecayProvider() *iotago.ManaDecayProvider {
	return v.manaDecayProvider
}

func (v *anyAPI) Encode(obj interface{}, opts ...serix.Option) ([]byte, error) {
	return v.serixAPI.Encode(context.TODO(), obj, opts...)
}

func (v *anyAPI) Decode(b []byte, obj interface{}, opts ...serix.Option) (int, error) {
	return v.serixAPI.Decode(context.TODO(), b, obj, opts...)
}
