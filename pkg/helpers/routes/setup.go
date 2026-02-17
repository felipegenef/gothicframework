package helpers

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

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
		router.Handle("/public/*", http.StripPrefix("/public/", http.FileServer(http.Dir("./public/"))))
	}

	router.Group(registerRoutes)
}
