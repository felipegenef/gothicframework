package data

import (
	"embed"
)

//go:embed .gothicCli/templates
var templatesFolder embed.FS

//go:embed server
var serverFolder embed.FS

//go:embed public
var publicFolder embed.FS

//go:embed src
var srcFolder embed.FS

//go:embed makefile
var makeFile embed.FS

//go:embed tailwind.config.js
var tailwindConfig embed.FS

//go:embed README.md
var readme embed.FS

//go:embed gothic-config.json
var goticConfig embed.FS

var env string = `HTTP_LISTEN_ADDR: ":8080"`

var gitIgnore string = `.env
bin
*_templ.go*
*templ.txt
node_modules
.aws-sam
tmp
optimize/*
public/styles.css
public/wasm/
.gothicCli/wasm-cache.json
.gothicCli/templ-cache.json
template.yaml
samconfig.toml
Dockerfile`

type GothicCliData struct {
	TemplateFiles                 map[string]embed.FS
	InitialFiles                  map[string]embed.FS
	PublicFolderAssets            map[string]embed.FS
	InitialDirs                   []string
	CustomTemplateBasedPages      map[string]string
	CustomTemplateBasedComponents map[string]string
	CustomTemplateBasedRoutes     map[string]string
	GitIgnore                     string
	Env                           string
	GoticConfig                   embed.FS
	Readme                        embed.FS
	MakeFile                      embed.FS
	SrcFolder                     embed.FS
	PublicFolder                  embed.FS
	ServerFolder                  embed.FS
	ProjectName                   string
	GoModName                     string
}

var DefaultCLIData = GothicCliData{
	PublicFolderAssets: map[string]embed.FS{
		"public/imageExample/blurred.png":  publicFolder,
		"public/imageExample/original.png": publicFolder,
		"public/favicon.ico":               publicFolder,
		"public/styles.css":                publicFolder,
	},
	TemplateFiles: map[string]embed.FS{
		".gothicCli/templates/Dockerfile-template":             templatesFolder,
		".gothicCli/templates/samconfig-template.toml":         templatesFolder,
		".gothicCli/templates/sam-template.yaml":               templatesFolder,
		".gothicCli/templates/routes_gen.go":                   templatesFolder,
		".gothicCli/templates/wasm/topic_gen.go":               templatesFolder,
		".gothicCli/templates/wasm/wasm_page_main.go":          templatesFolder,
		".gothicCli/templates/wasm/wasm_topic_manager_main.go": templatesFolder,
	},
	InitialFiles: map[string]embed.FS{
		// route files
		"src/routes/routes_gen.go": srcFolder,
		// page files
		"src/pages/index.templ":      srcFolder,
		"src/pages/revalidate.templ": srcFolder,
		"src/pages/counter.templ":    srcFolder,
		"src/pages/counter_templ.go": srcFolder,
		// topic files
		"src/topics/counter_topic.go": srcFolder,
		// layout files
		"src/layouts/layout.templ": srcFolder,
		// css files
		"src/css/app.css": srcFolder,
		// component files
		"src/components/helloWorld.templ": srcFolder,
		"src/components/lazyLoad.templ":   srcFolder,
		// api files
		"src/api/helloWorld.go": srcFolder,
		// root files
		"makefile":           makeFile,
		"tailwind.config.js": tailwindConfig,
		"README.md":          readme,
		"gothic-config.json": goticConfig,
	},
	InitialDirs: []string{
		// Root Dirs
		"public",
		".gothicCli",
		"src",
		"optimize",
		// Public Dirs
		"public/imageExample",
		// Cli Dirs
		".gothicCli/templates",
		".gothicCli/templates/wasm",
		// Src Dirs
		"src/api",
		"src/components",
		"src/css",
		"src/layouts",
		"src/pages",
		"src/routes",
		"src/topics",
	},
	GitIgnore:    gitIgnore,
	Env:          env,
	GoticConfig:  goticConfig,
	Readme:       readme,
	MakeFile:     makeFile,
	SrcFolder:    srcFolder,
	PublicFolder: publicFolder,
	ServerFolder: serverFolder,
	CustomTemplateBasedPages: map[string]string{
		"src/pages/revalidate.templ": "Revalidate",
		"src/pages/index.templ":      "Index",
		"src/pages/counter.templ":    "Counter",
	},
	CustomTemplateBasedComponents: map[string]string{
		"src/components/helloWorld.templ": "HelloWorld",
		"src/components/lazyLoad.templ":   "LazyLoad",
	},
	CustomTemplateBasedRoutes: map[string]string{
		"src/api/helloWorld.go": "HelloWorld",
	},
}
