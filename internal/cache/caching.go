package cache

import (
	"errors"
	"log/slog"
	"strings"
	"sync"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/protobuf/proto"
)

var ErrInvalidQuota = errors.New("cache: provider ID and quota are required")

type Cache struct {
	logger *slog.Logger
	cache  sync.Map
}

func NewCache(logger *slog.Logger) *Cache {
	return &Cache{
		logger: logger,
		cache:  sync.Map{},
	}
}

func (c *Cache) SaveQuota(id string, quota *aoyorouter.ProviderQuota) error {
	if c == nil || strings.TrimSpace(id) == "" || quota == nil {
		return ErrInvalidQuota
	}

	c.cache.Store(id, proto.Clone(quota).(*aoyorouter.ProviderQuota))
	return nil
}

func (c *Cache) GetQuota(id string) (*aoyorouter.ProviderQuota, bool) {
	if c == nil || strings.TrimSpace(id) == "" {
		return nil, false
	}

	value, ok := c.cache.Load(id)
	if !ok {
		return nil, false
	}

	quota, ok := value.(*aoyorouter.ProviderQuota)
	if !ok || quota == nil {
		c.cache.Delete(id)
		return nil, false
	}

	return proto.Clone(quota).(*aoyorouter.ProviderQuota), true
}
