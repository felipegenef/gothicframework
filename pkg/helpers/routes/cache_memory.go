package helpers

import (
	"sync"
	"time"
)

type memoryCacheEntry struct {
	data      []byte
	expiresAt time.Time
	hasTTL    bool
}

type InMemoryCacheStore struct {
	mu      sync.RWMutex
	entries map[string]memoryCacheEntry
	config  *CacheConfig
}

func NewInMemoryCacheStore(config *CacheConfig) *InMemoryCacheStore {
	return &InMemoryCacheStore{
		entries: make(map[string]memoryCacheEntry),
		config:  config,
	}
}

func (s *InMemoryCacheStore) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	entry, exists := s.entries[key]
	s.mu.RUnlock()

	if !exists {
		return nil, false
	}

	if entry.hasTTL && time.Now().After(entry.expiresAt) {
		s.mu.Lock()
		delete(s.entries, key)
		s.mu.Unlock()
		return nil, false
	}

	data := entry.data
	if s.compressionEnabled() {
		decompressed, err := decompressData(data, s.compressionMethod())
		if err != nil {
			return nil, false
		}
		data = decompressed
	}

	return data, true
}

func (s *InMemoryCacheStore) Set(key string, value []byte, ttl time.Duration) {
	data := value
	if s.compressionEnabled() {
		compressed, err := compressData(value, s.compressionMethod())
		if err == nil {
			data = compressed
		}
	}

	entry := memoryCacheEntry{data: data}
	if ttl > 0 {
		entry.hasTTL = true
		entry.expiresAt = time.Now().Add(ttl)
	}

	s.mu.Lock()
	s.entries[key] = entry
	s.mu.Unlock()
}

func (s *InMemoryCacheStore) Flush() error {
	s.mu.Lock()
	s.entries = make(map[string]memoryCacheEntry)
	s.mu.Unlock()
	return nil
}

func (s *InMemoryCacheStore) Close() error {
	return s.Flush()
}

func (s *InMemoryCacheStore) compressionEnabled() bool {
	return s.config != nil && s.config.Compression
}

func (s *InMemoryCacheStore) compressionMethod() CompressionMethod {
	if s.config != nil {
		return s.config.CompressionMethod
	}
	return GZIP
}
