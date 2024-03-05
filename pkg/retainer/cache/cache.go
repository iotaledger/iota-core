package cache

import (
	"github.com/VictoriaMetrics/fastcache"
)

type Cache struct {
	*fastcache.Cache
}

func NewCache(maxSize int) *Cache {
	return &Cache{
		Cache: fastcache.New(maxSize),
	}
}

func (c *Cache) Get(key []byte) []byte {
	if c.Has(key) {
		return c.Get(key)
	}

	return nil
}
