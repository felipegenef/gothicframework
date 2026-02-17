package api

import (
	"encoding/json"
	"net/http"

	routes "github.com/felipegenef/gothicframework/pkg/helpers/routes"
)

/**
 * 💡 Gothic Framework supports JSON-based API routes — similar to Next.js’s `/api` folder concept.
 *
 * You can define lightweight REST-style endpoints using `ApiRouteConfig` and a regular Go handler function.
 * This allows you to keep backend logic co-located with your frontend code while benefiting from serverless scalability.
 *
 * This file defines a single function: `HelloWorld`, which returns a simple JSON response.
 */

// HelloWorldResponse defines the structure of the JSON payload returned by the route.
// You can expand this with additional fields as needed.
type HelloWorldResponse struct {
	Message string `json:"message"`
}

/**
 * `HelloWorldConfig` registers this handler as an API route.
 *
 * - `HttpMethod`: Specifies that this endpoint handles HTTP GET requests.
 * - `Type`: Controls caching behavior — routes.DYNAMIC (default, no caching),
 *   routes.STATIC (cached forever), or routes.ISR (cached with TTL).
 * - `RevalidateInSec`: When Type is ISR, sets the TTL in seconds for cache entries.
 *
 * Example with ISR caching:
 *   var MyConfig = routes.ApiRouteConfig{
 *       HttpMethod:      routes.GET,
 *       Type:            routes.ISR,
 *       RevalidateInSec: 60,
 *   }
 */
var HelloWorldConfig = routes.ApiRouteConfig{
	HttpMethod: routes.GET,
	Type:       routes.DYNAMIC,
}

/**
 * `HelloWorld` is a simple HTTP handler that returns a JSON object with a message.
 *
 * This is the only function in this file and serves as an example of Gothic’s JSON API capabilities.
 *
 * Response:
 * {
 *   "message": "Hello World from GOTH API ROUTE"
 * }
 */
func HelloWorld(w http.ResponseWriter, r *http.Request) {
	response, _ := json.Marshal(HelloWorldResponse{
		Message: "Hello World from GOTH API ROUTE",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}
