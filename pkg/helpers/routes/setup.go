package helpers

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

func noCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		// Strip conditional headers so FileServer always returns 200 with fresh content.
		// Without this, FileServer honors If-None-Match / If-Modified-Since and returns
		// 304, causing the browser to use a stale cached version of CSS/JS/WASM files.
		r.Header.Del("If-None-Match")
		r.Header.Del("If-Modified-Since")
		next.ServeHTTP(w, r)
	})
}

// wasmEncodedResponseWriter forces Content-Type: application/wasm and the
// correct Content-Encoding for pre-compressed WASM files served by http.FileServer.
type wasmEncodedResponseWriter struct {
	http.ResponseWriter
	encoding string
}

func (w *wasmEncodedResponseWriter) Header() http.Header {
	h := w.ResponseWriter.Header()
	h.Set("Content-Type", "application/wasm")
	h.Set("Content-Encoding", w.encoding)
	return h
}

// wasmAwareFileServer wraps a file server so that .wasm.gz and .wasm.br requests
// get the correct Content-Type and Content-Encoding headers.
func wasmAwareFileServer(fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".wasm.gz"):
			fileServer.ServeHTTP(&wasmEncodedResponseWriter{w, "gzip"}, r)
		case strings.HasSuffix(r.URL.Path, ".wasm.br"):
			fileServer.ServeHTTP(&wasmEncodedResponseWriter{w, "br"}, r)
		default:
			fileServer.ServeHTTP(w, r)
		}
	})
}

// Setup initializes caching, static file serving, and registers routes.
// It reads the GOTHIC_MODE environment variable to determine dev vs production mode.
// In dev mode (GOTHIC_MODE=dev), LocalDevelopmentCache is used; in production, CacheStrategy is used.
func Setup(router chi.Router, config AppConfig, registerRoutes func(chi.Router)) {
	isDev := os.Getenv("GOTHIC_MODE") == "dev"

	cacheType := config.CacheStrategy
	if isDev {
		cacheType = config.LocalDevelopmentCache
		if cacheType == CACHE_CONTROL_HEADERS {
			cacheType = IN_MEMORY
		}
	}
	InitCache(cacheType, config.CacheConfig)

	if isDev {
		if store := getGlobalCacheStore(); store != nil {
			store.Flush()
		}
	}

	if config.ServeStaticFiles == ALL_ENVS || isDev {
		slog.Info("application serving local public folder")
		fileServer := http.StripPrefix("/public/", http.FileServer(http.Dir("./public/")))
		wasmServer := wasmAwareFileServer(fileServer)
		if isDev {
			router.Handle("/public/*", noCacheMiddleware(wasmServer))
		} else {
			router.Handle("/public/*", wasmServer)
		}
	}

	router.Group(registerRoutes)
}
