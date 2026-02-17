package helpers

import (
	"log"
	"sync"
	"time"
)

type CacheType int

const (
	CACHE_CONTROL_HEADERS CacheType = iota // Default — production behavior (Cache-Control headers for CDN)
	IN_MEMORY                              // Go in-memory map
	REDIS                                  // Redis-backed
	LOCAL_FILES                            // File-system-backed
)

type StaticFilesMode int

const (
	HOT_RELOAD_ONLY StaticFilesMode = iota // Serve /public/* only during development
	ALL_ENVS                               // Serve /public/* in all environments (requires public folder at runtime)
)

type CompressionMethod int

const (
	GZIP   CompressionMethod = iota // Default compression method
	BROTLI                          // Brotli compression
)

type AppConfig struct {
	// CacheStrategy selects the production cache backend.
	CacheStrategy CacheType

	// LocalDevelopmentCache selects the dev cache backend. Default: IN_MEMORY.
	LocalDevelopmentCache CacheType

	// ServeStaticFiles controls when /public/* is served from disk.
	// HOT_RELOAD_ONLY (default): only during development.
	// ALL_ENVS: all environments (public folder must be present alongside the binary).
	ServeStaticFiles StaticFilesMode

	// CacheConfig provides backend-specific settings (Redis URL, file path, compression).
	CacheConfig *CacheConfig
}

type CacheConfig struct {
	RedisURL          string
	RedisPassword     string
	RedisTLS          bool
	CacheFilesPath    string
	Compression       bool
	CompressionMethod CompressionMethod
}

type CacheStore interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
	Flush() error
	Close() error
}

var (
	globalMu         sync.Mutex
	globalCacheStore CacheStore
	globalCacheType  CacheType
	globalInitDone   bool
)

// InitCache explicitly initializes the global cache store with the given type and config.
// This should be called from the server entry point before routes are registered.
// If called multiple times, only the first call takes effect.
func InitCache(cacheType CacheType, config *CacheConfig) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalInitDone {
		return
	}

	globalCacheType = cacheType
	globalCacheStore = buildCacheStore(cacheType, config)
	globalInitDone = true
}

// getGlobalCacheStore returns the global CacheStore, lazily initializing with CACHE_CONTROL_HEADERS defaults if needed.
func getGlobalCacheStore() CacheStore {
	globalMu.Lock()
	defer globalMu.Unlock()

	if !globalInitDone {
		globalCacheType = CACHE_CONTROL_HEADERS
		globalCacheStore = buildCacheStore(CACHE_CONTROL_HEADERS, nil)
		globalInitDone = true
	}
	return globalCacheStore
}

// getGlobalCacheType returns the global CacheType, lazily initializing with CACHE_CONTROL_HEADERS defaults if needed.
func getGlobalCacheType() CacheType {
	globalMu.Lock()
	defer globalMu.Unlock()

	if !globalInitDone {
		globalCacheType = CACHE_CONTROL_HEADERS
		globalCacheStore = buildCacheStore(CACHE_CONTROL_HEADERS, nil)
		globalInitDone = true
	}
	return globalCacheType
}

// resetGlobalCache resets the global cache state. Used in tests for isolation.
func resetGlobalCache() {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalCacheStore != nil {
		globalCacheStore.Close()
	}
	globalCacheStore = nil
	globalCacheType = CACHE_CONTROL_HEADERS
	globalInitDone = false
}

func buildCacheStore(cacheType CacheType, cacheConfig *CacheConfig) CacheStore {
	switch cacheType {
	case IN_MEMORY:
		return NewInMemoryCacheStore(cacheConfig)
	case REDIS:
		store, err := NewRedisCacheStore(cacheConfig)
		if err != nil {
			log.Printf("gothic: failed to initialize Redis cache, falling back to in-memory: %v", err)
			return NewInMemoryCacheStore(cacheConfig)
		}
		return store
	case LOCAL_FILES:
		store, err := NewLocalFilesCacheStore(cacheConfig)
		if err != nil {
			log.Printf("gothic: failed to initialize file cache, falling back to in-memory: %v", err)
			return NewInMemoryCacheStore(cacheConfig)
		}
		return store
	default:
		// CACHE_CONTROL_HEADERS or unknown: return in-memory as a no-op placeholder.
		// CACHE_CONTROL_HEADERS handlers set Cache-Control headers and skip the store.
		return NewInMemoryCacheStore(cacheConfig)
	}
}
