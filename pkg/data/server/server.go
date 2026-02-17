{{.MainServerPackageName}}

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"{{.GoModName}}/src/routes"
	gothicComponents "github.com/felipegenef/gothicframework/components"
	gothicRoutes "github.com/felipegenef/gothicframework/pkg/helpers/routes"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func {{.MainServerFunctionName}} {
	godotenv.Load()

	router := chi.NewMux()
	router.Use(middleware.Logger)

	/**
	*                         Gothic App Configuration
	*
	* Setup initializes caching, static file serving, and file-based routes.
	*
	* - CacheStrategy: The production cache backend.
	*   Options: CACHE_CONTROL_HEADERS (default), IN_MEMORY, REDIS, LOCAL_FILES
	*
	* - LocalDevelopmentCache: The cache backend used during hot-reload development.
	*   Options: IN_MEMORY (default), REDIS, LOCAL_FILES
	*
	* - ServeStaticFiles: Controls when public assets (css, js, images) are served from disk.
	*   HOT_RELOAD_ONLY (default): Only during development. In production, AWS CloudFront
	*     serves files from an S3 bucket origin.
	*   ALL_ENVS: Serves files from disk in all environments. The public folder must be
	*     present alongside the server binary (e.g., COPY into Docker container).
	*
	* - CacheConfig: Optional backend-specific settings (Redis URL, compression, etc.)
	*
	 */
	gothicRoutes.Setup(router, gothicRoutes.AppConfig{
		CacheStrategy:         gothicRoutes.CACHE_CONTROL_HEADERS,
		LocalDevelopmentCache: gothicRoutes.IN_MEMORY,
		ServeStaticFiles:      gothicRoutes.HOT_RELOAD_ONLY,
	}, routes.RegisterFileBasedRoutes)

	/**
	*                            OptimizedImage Component
	*
	* This component implements lazy loading with a smooth transition from a low-res placeholder
	* to the full-resolution image — improving perceived performance and SEO.
	*
	* How it works:
	* - When `IsFirstLoad` is `true` (from initial page render, e.g., in `Index`):
	*   - A blurred image is shown using a smaller version.
	*   - `hx-get` fetches the full-res version in the background.
	*   - On load, the image is swapped in place using HTMX.
	*
	* - When `IsFirstLoad` is `false` (in HTMX request):
	*   - The full-resolution image is rendered immediately.
	*
	* Tip: To see this in action, check how the `Index` page uses `OptimizedImage`.
	*/
	gothicComponents.OptimizedImageConfig.RegisterRoute(router,"/optimizedImage/{name}/{extension}",gothicComponents.OptimizedImage)

	port := os.Getenv("HTTP_LISTEN_ADDR")
	slog.Info("application running", "port", port)
	log.Fatal(http.ListenAndServe(port, router))
}
