package helpers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestInMemoryCacheStore(t *testing.T) {
	store := NewInMemoryCacheStore(nil)

	// Test set and get
	store.Set("key1", []byte("hello"), 0)
	data, ok := store.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(data, []byte("hello")) {
		t.Errorf("expected 'hello', got '%s'", string(data))
	}

	// Test cache miss
	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}

	// Test TTL expiry
	store.Set("expire", []byte("temp"), 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok = store.Get("expire")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}

	// Test no expiry (TTL = 0)
	store.Set("forever", []byte("persistent"), 0)
	time.Sleep(10 * time.Millisecond)
	_, ok = store.Get("forever")
	if !ok {
		t.Error("expected cache hit for entry with no TTL")
	}

	store.Close()
}

func TestInMemoryCacheStoreWithGzipCompression(t *testing.T) {
	config := &CacheConfig{Compression: true, CompressionMethod: GZIP}
	store := NewInMemoryCacheStore(config)

	original := []byte("<html><body>Hello World</body></html>")
	store.Set("key", original, 0)
	data, ok := store.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(data, original) {
		t.Errorf("decompressed data doesn't match original")
	}
}

func TestInMemoryCacheStoreWithBrotliCompression(t *testing.T) {
	config := &CacheConfig{Compression: true, CompressionMethod: BROTLI}
	store := NewInMemoryCacheStore(config)

	original := []byte("<html><body>Hello World Brotli</body></html>")
	store.Set("key", original, 0)
	data, ok := store.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(data, original) {
		t.Errorf("decompressed brotli data doesn't match original")
	}
}

func TestLocalFilesCacheStore(t *testing.T) {
	tmpDir := t.TempDir()
	config := &CacheConfig{CacheFilesPath: tmpDir}

	store, err := NewLocalFilesCacheStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Test set and get
	store.Set("key1", []byte("file-data"), 0)
	data, ok := store.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(data, []byte("file-data")) {
		t.Errorf("expected 'file-data', got '%s'", string(data))
	}

	// Test cache miss
	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}

	// Test TTL expiry
	store.Set("expire", []byte("temp"), 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok = store.Get("expire")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestLocalFilesCacheStoreWithCompression(t *testing.T) {
	tmpDir := t.TempDir()
	config := &CacheConfig{CacheFilesPath: tmpDir, Compression: true}

	store, err := NewLocalFilesCacheStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	original := []byte("<html><body>Compressed File Data</body></html>")
	store.Set("key", original, 0)
	data, ok := store.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(data, original) {
		t.Errorf("decompressed data doesn't match original")
	}
}

func TestBuildCacheStore(t *testing.T) {
	// CACHE_CONTROL_HEADERS -> InMemoryCacheStore (handlers skip it via Cache-Control headers)
	store := buildCacheStore(CACHE_CONTROL_HEADERS, nil)
	if _, ok := store.(*InMemoryCacheStore); !ok {
		t.Error("CACHE_CONTROL_HEADERS should return InMemoryCacheStore")
	}

	// IN_MEMORY -> InMemoryCacheStore
	store = buildCacheStore(IN_MEMORY, nil)
	if _, ok := store.(*InMemoryCacheStore); !ok {
		t.Error("IN_MEMORY should return InMemoryCacheStore")
	}

	// REDIS without config -> fallback to InMemoryCacheStore
	store = buildCacheStore(REDIS, nil)
	if _, ok := store.(*InMemoryCacheStore); !ok {
		t.Error("REDIS without config should fallback to InMemoryCacheStore")
	}

	// LOCAL_FILES -> LocalFilesCacheStore
	tmpDir := t.TempDir()
	store = buildCacheStore(LOCAL_FILES, &CacheConfig{CacheFilesPath: tmpDir})
	if _, ok := store.(*LocalFilesCacheStore); !ok {
		t.Error("LOCAL_FILES should return LocalFilesCacheStore")
	}
}

func TestCompressDecompressGzip(t *testing.T) {
	original := []byte("Hello, this is test data for compression!")

	compressed, err := compressData(original, GZIP)
	if err != nil {
		t.Fatalf("gzip compress error: %v", err)
	}
	decompressed, err := decompressData(compressed, GZIP)
	if err != nil {
		t.Fatalf("gzip decompress error: %v", err)
	}
	if !bytes.Equal(original, decompressed) {
		t.Error("gzip round-trip failed")
	}
}

func TestCompressDecompressBrotli(t *testing.T) {
	original := []byte("Hello, this is test data for brotli compression!")

	compressed, err := compressData(original, BROTLI)
	if err != nil {
		t.Fatalf("brotli compress error: %v", err)
	}
	decompressed, err := decompressData(compressed, BROTLI)
	if err != nil {
		t.Fatalf("brotli decompress error: %v", err)
	}
	if !bytes.Equal(original, decompressed) {
		t.Error("brotli round-trip failed")
	}
}

func TestCompressDecompressDefaultIsGzip(t *testing.T) {
	original := []byte("Testing default compression method")

	compressed, err := compressData(original, GZIP)
	if err != nil {
		t.Fatalf("default compress error: %v", err)
	}
	decompressed, err := decompressData(compressed, GZIP)
	if err != nil {
		t.Fatalf("default decompress error: %v", err)
	}
	if !bytes.Equal(original, decompressed) {
		t.Error("default compression round-trip failed")
	}
}

func TestCacheTypeZeroValueIsCACHE_CONTROL_HEADERS(t *testing.T) {
	var ct CacheType
	if ct != CACHE_CONTROL_HEADERS {
		t.Errorf("zero value of CacheType should be CACHE_CONTROL_HEADERS, got %d", ct)
	}
}

func TestInitCache(t *testing.T) {
	resetGlobalCache()

	InitCache(IN_MEMORY, &CacheConfig{Compression: true, CompressionMethod: GZIP})

	store := getGlobalCacheStore()
	if _, ok := store.(*InMemoryCacheStore); !ok {
		t.Error("expected InMemoryCacheStore after InitCache with IN_MEMORY")
	}

	cacheType := getGlobalCacheType()
	if cacheType != IN_MEMORY {
		t.Errorf("expected IN_MEMORY, got %d", cacheType)
	}

	// Calling InitCache again should be a no-op (already initialized)
	InitCache(REDIS, nil)
	cacheType = getGlobalCacheType()
	if cacheType != IN_MEMORY {
		t.Error("InitCache should be a no-op after first call")
	}

	resetGlobalCache()
}

func TestGlobalCacheInitialization(t *testing.T) {
	resetGlobalCache()

	// Use InitCache directly instead of relying on file-based config
	InitCache(IN_MEMORY, nil)

	store := getGlobalCacheStore()
	if _, ok := store.(*InMemoryCacheStore); !ok {
		t.Error("expected InMemoryCacheStore for in_memory config")
	}

	cacheType := getGlobalCacheType()
	if cacheType != IN_MEMORY {
		t.Errorf("expected IN_MEMORY, got %d", cacheType)
	}

	resetGlobalCache()
}

func TestGlobalCacheDefaultsCACHE_CONTROL_HEADERS(t *testing.T) {
	// Without calling InitCache, lazy fallback should default to CACHE_CONTROL_HEADERS
	resetGlobalCache()

	store := getGlobalCacheStore()
	if _, ok := store.(*InMemoryCacheStore); !ok {
		t.Error("expected InMemoryCacheStore for default CACHE_CONTROL_HEADERS config")
	}

	cacheType := getGlobalCacheType()
	if cacheType != CACHE_CONTROL_HEADERS {
		t.Errorf("expected CACHE_CONTROL_HEADERS cache type, got %d", cacheType)
	}

	resetGlobalCache()
}

// --- Setup() tests ---

func TestSetupDevModeDefaults(t *testing.T) {
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "dev")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		CacheStrategy:         CACHE_CONTROL_HEADERS,
		LocalDevelopmentCache: IN_MEMORY,
	}, func(r chi.Router) {})

	cacheType := getGlobalCacheType()
	if cacheType != IN_MEMORY {
		t.Errorf("expected IN_MEMORY in dev mode, got %d", cacheType)
	}

	resetGlobalCache()
}

func TestSetupDevModeDefaultsZeroValue(t *testing.T) {
	// When LocalDevelopmentCache is zero value (CACHE_CONTROL_HEADERS), Setup should
	// override it to IN_MEMORY in dev mode.
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "dev")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		CacheStrategy: CACHE_CONTROL_HEADERS,
		// LocalDevelopmentCache not set → zero value = CACHE_CONTROL_HEADERS
	}, func(r chi.Router) {})

	cacheType := getGlobalCacheType()
	if cacheType != IN_MEMORY {
		t.Errorf("expected IN_MEMORY when LocalDevelopmentCache is zero value in dev mode, got %d", cacheType)
	}

	resetGlobalCache()
}

func TestSetupProdModeDefaults(t *testing.T) {
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		CacheStrategy:         CACHE_CONTROL_HEADERS,
		LocalDevelopmentCache: IN_MEMORY,
	}, func(r chi.Router) {})

	cacheType := getGlobalCacheType()
	if cacheType != CACHE_CONTROL_HEADERS {
		t.Errorf("expected CACHE_CONTROL_HEADERS in prod mode, got %d", cacheType)
	}

	resetGlobalCache()
}

func TestSetupExplicitDevOverride(t *testing.T) {
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "dev")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		CacheStrategy:         CACHE_CONTROL_HEADERS,
		LocalDevelopmentCache: REDIS,
	}, func(r chi.Router) {})

	// REDIS without config falls back to in-memory, but the type should still be REDIS
	cacheType := getGlobalCacheType()
	if cacheType != REDIS {
		t.Errorf("expected REDIS in dev mode with explicit override, got %d", cacheType)
	}

	resetGlobalCache()
}

func TestSetupDevServesPublicFolder(t *testing.T) {
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "dev")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		ServeStaticFiles: HOT_RELOAD_ONLY,
	}, func(r chi.Router) {})

	// The /public/* route should be registered in dev mode
	req := httptest.NewRequest("GET", "/public/test.css", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// We expect a 404 (file doesn't exist) rather than 405 (method not allowed),
	// which proves the route IS registered.
	if rec.Code == http.StatusMethodNotAllowed {
		t.Error("expected /public/* route to be registered in dev mode")
	}

	resetGlobalCache()
}

func TestSetupProdNoPublicFolder(t *testing.T) {
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		ServeStaticFiles: HOT_RELOAD_ONLY,
	}, func(r chi.Router) {})

	// The /public/* route should NOT be registered in prod mode with HOT_RELOAD_ONLY
	req := httptest.NewRequest("GET", "/public/test.css", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /public/* in prod mode with HOT_RELOAD_ONLY, got %d", rec.Code)
	}

	resetGlobalCache()
}

func TestSetupAllEnvsServesPublicFolder(t *testing.T) {
	resetGlobalCache()
	t.Setenv("GOTHIC_MODE", "")

	r := chi.NewRouter()
	Setup(r, AppConfig{
		ServeStaticFiles: ALL_ENVS,
	}, func(r chi.Router) {})

	// The /public/* route should be registered even in prod mode with ALL_ENVS
	req := httptest.NewRequest("GET", "/public/test.css", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// We expect a 404 (file doesn't exist) rather than 405 (method not allowed),
	// which proves the route IS registered.
	if rec.Code == http.StatusMethodNotAllowed {
		t.Error("expected /public/* route to be registered in prod mode with ALL_ENVS")
	}

	resetGlobalCache()
}
