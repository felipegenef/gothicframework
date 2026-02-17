package helpers

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCacheStore struct {
	client *redis.Client
	config *CacheConfig
}

func NewRedisCacheStore(config *CacheConfig) (*RedisCacheStore, error) {
	if config == nil || config.RedisURL == "" {
		return nil, fmt.Errorf("RedisURL is required for REDIS cache type")
	}

	opts := &redis.Options{
		Addr:     config.RedisURL,
		Password: config.RedisPassword,
	}
	if config.RedisTLS {
		opts.TLSConfig = &tls.Config{}
	}
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", config.RedisURL, err)
	}

	return &RedisCacheStore{
		client: client,
		config: config,
	}, nil
}

func (s *RedisCacheStore) Get(key string) ([]byte, bool) {
	ctx := context.Background()
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	if s.compressionEnabled() {
		decompressed, err := decompressData(data, s.compressionMethod())
		if err != nil {
			return nil, false
		}
		data = decompressed
	}

	return data, true
}

func (s *RedisCacheStore) Set(key string, value []byte, ttl time.Duration) {
	data := value
	if s.compressionEnabled() {
		compressed, err := compressData(value, s.compressionMethod())
		if err == nil {
			data = compressed
		}
	}

	ctx := context.Background()
	s.client.Set(ctx, key, data, ttl)
}

func (s *RedisCacheStore) Flush() error {
	return s.client.FlushDB(context.Background()).Err()
}

func (s *RedisCacheStore) Close() error {
	return s.client.Close()
}

func (s *RedisCacheStore) compressionEnabled() bool {
	return s.config != nil && s.config.Compression
}

func (s *RedisCacheStore) compressionMethod() CompressionMethod {
	if s.config != nil {
		return s.config.CompressionMethod
	}
	return GZIP
}
