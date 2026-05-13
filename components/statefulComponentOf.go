package gothicComponents

import (
	routes "github.com/felipegenef/gothicframework/pkg/helpers/routes"
	"github.com/a-h/templ"
)

// StatefulComponentOf returns a lazy-loading HTMX wrapper for any component
// that has a registered route. The path is read from RouteConfig.Path, which
// is set automatically by RegisterRoute at server startup — no magic strings.
func StatefulComponentOf[T any](config *routes.RouteConfig[T]) templ.Component {
	return StatefulComponent(config.Path)
}
