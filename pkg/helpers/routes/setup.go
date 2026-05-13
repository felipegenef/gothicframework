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
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

// wasmGzipResponseWriter forces the correct Content-Type and Content-Encoding
// for pre-compressed .wasm.gz files served by http.FileServer.
// Without this, FileServer sets Content-Type: application/gzip (wrong) and the
// browser treats the file as a download instead of WASM.
type wasmGzipResponseWriter struct {
	http.ResponseWriter
}

func (w *wasmGzipResponseWriter) Header() http.Header {
	h := w.ResponseWriter.Header()
	h.Set("Content-Type", "application/wasm")
	h.Set("Content-Encoding", "gzip")
	return h
}

// wasmAwareFileServer wraps a file server so that .wasm.gz requests get the
// correct Content-Type and Content-Encoding headers.
func wasmAwareFileServer(fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".wasm.gz") {
			fileServer.ServeHTTP(&wasmGzipResponseWriter{w}, r)
			return
		}
		fileServer.ServeHTTP(w, r)
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
