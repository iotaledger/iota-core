package cache

import (
	"github.com/VictoriaMetrics/fastcache"
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var validatorResponsesTypeSettings = serix.TypeSettings{}.WithLengthPrefixType(serix.LengthPrefixTypeAsByte)

type Cache struct {
	cache *fastcache.Cache
}

func NewCache(maxSize int) *Cache {
	return &Cache{
		cache: fastcache.New(maxSize),
	}
}

func (c *Cache) Set(key, value []byte) {
	c.cache.Set(key, value)
}

func (c *Cache) Get(key []byte) []byte {
	if c.cache.Has(key) {
		value := make([]byte, 0)

		return c.cache.Get(value, key)
	}

	return nil
}

func (c *Cache) Reset() {
	c.cache.Reset()
}

func (c *Cache) GetOrCreateRegisteredValidators(apiForEpoch iotago.API, key []byte, defaultValueFunc func() ([]*api.ValidatorResponse, error)) ([]*api.ValidatorResponse, error) {
	registeredValidatorsBytes := c.Get(key)
	if registeredValidatorsBytes == nil {
		// get the ordered registered validators list from engine.
		registeredValidators, err := defaultValueFunc()
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrNotFound, "ordered registered validators list not found: %w", err)
		}

		// store validator responses in cache.
		registeredValidatorsBytes, err := apiForEpoch.Encode(registeredValidators, serix.WithTypeSettings(validatorResponsesTypeSettings))
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to encode registered validators list: %w", err)
		}

		c.Set(key, registeredValidatorsBytes)

		return registeredValidators, nil
	}

	validatorResp := make([]*api.ValidatorResponse, 0)
	_, err := apiForEpoch.Decode(registeredValidatorsBytes, &validatorResp, serix.WithTypeSettings(validatorResponsesTypeSettings))
	if err != nil {
		return nil, err
	}

	return validatorResp, nil
}
