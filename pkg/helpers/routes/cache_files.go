package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LocalFilesCacheStore struct {
	basePath string
	config   *CacheConfig
	mu       sync.RWMutex
	ttlMap   map[string]time.Time
}

func NewLocalFilesCacheStore(config *CacheConfig) (*LocalFilesCacheStore, error) {
	basePath := ".gothic-cache"
	if config != nil && config.CacheFilesPath != "" {
		basePath = config.CacheFilesPath
	}

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", basePath, err)
	}

	return &LocalFilesCacheStore{
		basePath: basePath,
		config:   config,
		ttlMap:   make(map[string]time.Time),
	}, nil
}

func (s *LocalFilesCacheStore) hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func (s *LocalFilesCacheStore) filePath(key string) string {
	return filepath.Join(s.basePath, s.hashKey(key))
}

func (s *LocalFilesCacheStore) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	expiresAt, hasTTL := s.ttlMap[key]
	s.mu.RUnlock()

	if hasTTL && time.Now().After(expiresAt) {
		s.mu.Lock()
		delete(s.ttlMap, key)
		s.mu.Unlock()
		os.Remove(s.filePath(key))
		return nil, false
	}

	data, err := os.ReadFile(s.filePath(key))
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

func (s *LocalFilesCacheStore) Set(key string, value []byte, ttl time.Duration) {
	data := value
	if s.compressionEnabled() {
		compressed, err := compressData(value, s.compressionMethod())
		if err == nil {
			data = compressed
		}
	}

	if err := os.WriteFile(s.filePath(key), data, 0644); err != nil {
		return
	}

	if ttl > 0 {
		s.mu.Lock()
		s.ttlMap[key] = time.Now().Add(ttl)
		s.mu.Unlock()
	}
}

func (s *LocalFilesCacheStore) Flush() error {
	s.mu.Lock()
	s.ttlMap = make(map[string]time.Time)
	s.mu.Unlock()
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		os.Remove(filepath.Join(s.basePath, entry.Name()))
	}
	return nil
}

func (s *LocalFilesCacheStore) Close() error {
	return s.Flush()
}

func (s *LocalFilesCacheStore) compressionEnabled() bool {
	return s.config != nil && s.config.Compression
}

func (s *LocalFilesCacheStore) compressionMethod() CompressionMethod {
	if s.config != nil {
		return s.config.CompressionMethod
	}
	return GZIP
}
