package helpers

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempGo(t *testing.T, name, src string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestAstScanFile_PackageName(t *testing.T) {
	p := writeTempGo(t, "x.go", "package mypkg\n")
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.PackageName != "mypkg" {
		t.Errorf("PackageName: got %q, want %q", res.PackageName, "mypkg")
	}
}

func TestAstScanFile_RouteConfigGenericMultiline(t *testing.T) {
	// The old regex required single-line, no comments. AST handles whitespace
	// and multi-line bodies.
	src := `package pages

import (
	routes "example.com/routes"
)

var MyConfig = routes.RouteConfig[Props]{
	Type:       routes.STATIC,
	HttpMethod: routes.GET,
}

type Props struct{}
`
	p := writeTempGo(t, "p_templ.go", src)
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.RouteConfigName != "MyConfig" {
		t.Errorf("RouteConfigName: got %q, want %q", res.RouteConfigName, "MyConfig")
	}
}

func TestAstScanFile_ApiRouteConfig(t *testing.T) {
	src := `package api

import routes "example.com/routes"

var MyApi = routes.ApiRouteConfig{
	HttpMethod: routes.POST,
}
`
	p := writeTempGo(t, "a.go", src)
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.ApiRouteConfigName != "MyApi" {
		t.Errorf("ApiRouteConfigName: got %q, want %q", res.ApiRouteConfigName, "MyApi")
	}
}

func TestAstScanFile_RouteConfigInsideCommentIgnored(t *testing.T) {
	// A Go comment containing a fake var must NOT be picked up.
	src := `package pages

// var Fake = routes.RouteConfig[Props]{}

/*
var AlsoFake = routes.RouteConfig[Props]{}
*/
`
	p := writeTempGo(t, "p_templ.go", src)
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.RouteConfigName != "" {
		t.Errorf("RouteConfigName: got %q, want empty (declared in comment)", res.RouteConfigName)
	}
}

func TestAstScanFile_RouteFuncTemplComponent(t *testing.T) {
	src := `package pages

import (
	"github.com/a-h/templ"
)

func Home(props any) templ.Component {
	return nil
}

func helper() {}
`
	p := writeTempGo(t, "home_templ.go", src)
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.RouteFuncName != "Home" {
		t.Errorf("RouteFuncName: got %q, want %q", res.RouteFuncName, "Home")
	}
	if res.ApiFuncName != "helper" {
		t.Errorf("ApiFuncName: got %q, want %q (first no-return func)", res.ApiFuncName, "helper")
	}
}

func TestAstScanFile_ApiFunc(t *testing.T) {
	// First top-level no-return func is the api handler — matching the old regex
	// `^func\s+(\w+)\s*\(.*\)\s*{` which took the first match.
	src := `package api

import "net/http"

func Handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}
`
	p := writeTempGo(t, "a.go", src)
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.ApiFuncName != "Handler" {
		t.Errorf("ApiFuncName: got %q, want %q", res.ApiFuncName, "Handler")
	}
}

func TestAstScanFile_MethodsAreSkipped(t *testing.T) {
	// Methods (func with receiver) must not be confused with top-level funcs.
	src := `package pages

import "github.com/a-h/templ"

type R struct{}
func (r *R) Method() {}

func Page() templ.Component { return nil }
`
	p := writeTempGo(t, "p_templ.go", src)
	res, err := astScanFile(p)
	if err != nil {
		t.Fatalf("astScanFile: %v", err)
	}
	if res.RouteFuncName != "Page" {
		t.Errorf("RouteFuncName: got %q, want %q", res.RouteFuncName, "Page")
	}
	if res.ApiFuncName != "" {
		t.Errorf("ApiFuncName: got %q, want empty (only method with no return existed)", res.ApiFuncName)
	}
}

func TestAstScanFile_ParseErrorReturnsError(t *testing.T) {
	p := writeTempGo(t, "broken.go", "package pages\n\nthis is not valid go\n")
	if _, err := astScanFile(p); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}
