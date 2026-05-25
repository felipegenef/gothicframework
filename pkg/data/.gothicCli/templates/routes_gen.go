package routes
/**
* Auto-generated code during deployment. Do not modify this section directly.
* This file is auto generatade every time a ".go" or ".templ" file is changed
* in '/src/pages' "/src/components" or "/src/api" folders
*
*/
import (
	{{- if .ImportDefault }}
	routes "github.com/felipegenef/gothicframework/pkg/helpers/routes"
	{{- end }}
	{{ range .Imports }}
	{{.Package}} "{{.PackagePath}}"
	{{ end }}

	"github.com/go-chi/chi/v5"
)


func RegisterFileBasedRoutes(r chi.Router) {
	{{ range .Routes }}
		{{.ConfigPackageName}}.{{.ConfigName}}.RegisterRoute(r,"{{.HttpPath}}",{{.PackageName}}.{{.FunctionName}})
	{{ end }}
	{{ range .ApiRoutes }}
		{{.ConfigPackageName}}.{{.ConfigName}}.RegisterRoute(r,"{{.HttpPath}}",{{.PackageName}}.{{.FunctionName}})
	{{ end }}

}
