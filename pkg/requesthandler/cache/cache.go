package cache

import (
	"github.com/VictoriaMetrics/fastcache"
)

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
