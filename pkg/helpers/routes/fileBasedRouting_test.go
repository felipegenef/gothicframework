package helpers

import (
	"testing"
)

func TestNormalizeHttpPath(t *testing.T) {
	helper := NewFileBasedRouteHelper()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "pages index route",
			path:     "src/pages/index_templ.go",
			expected: "/",
		},
		{
			name:     "pages about route",
			path:     "src/pages/about_templ.go",
			expected: "/about",
		},
		{
			name:     "pages nested route",
			path:     "src/pages/blog/post_templ.go",
			expected: "/blog/post",
		},
		{
			name:     "pages nested index route",
			path:     "src/pages/blog/index_templ.go",
			expected: "/blog",
		},
		{
			name:     "pages dynamic route param",
			path:     "src/pages/blog/var_id_templ.go",
			expected: "/blog/{id}",
		},
		{
			name:     "components route",
			path:     "src/components/navbar_templ.go",
			expected: "/components/navbar",
		},
		{
			name:     "api route with .go extension",
			path:     "src/api/users.go",
			expected: "/api/users",
		},
		{
			name:     "api nested route",
			path:     "src/api/v1/health.go",
			expected: "/api/v1/health",
		},
		{
			name:     "api dynamic route param",
			path:     "src/api/users/var_id.go",
			expected: "/api/users/{id}",
		},
		{
			name:     "api nested dynamic route param",
			path:     "src/api/v1/posts/var_postId.go",
			expected: "/api/v1/posts/{postId}",
		},
		{
			name:     "deeply nested pages",
			path:     "src/pages/admin/settings/profile_templ.go",
			expected: "/admin/settings/profile",
		},
		{
			name:     "root index only",
			path:     "src/pages/index_templ.go",
			expected: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := helper.normalizeHttpPath(tt.path)
			if got != tt.expected {
				t.Errorf("normalizeHttpPath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestRemoveDuplicates(t *testing.T) {
	helper := NewFileBasedRouteHelper()
	helper.TemplateInfo.Imports = []Imports{
		{Package: "pages", PackagePath: "example.com/src/pages"},
		{Package: "pages", PackagePath: "example.com/src/pages"},
		{Package: "components", PackagePath: "example.com/src/components"},
	}
	helper.TemplateInfo.Routes = []RouteTemplate{
		{ConfigName: "DefaultConfig", PackageName: "pages"},
	}

	helper.RemoveDuplicates()

	if !helper.TemplateInfo.ImportDefault {
		t.Error("expected ImportDefault to be true when DefaultConfig is used")
	}

	if len(helper.TemplateInfo.Imports) != 2 {
		t.Errorf("expected 2 unique imports, got %d", len(helper.TemplateInfo.Imports))
	}
}

func TestInitialize(t *testing.T) {
	helper := NewFileBasedRouteHelper()
	helper.TemplateInfo.Routes = []RouteTemplate{{FunctionName: "old"}}
	helper.TemplateInfo.ApiRoutes = []RouteTemplate{{FunctionName: "old"}}
	helper.TemplateInfo.ImportDefault = true

	helper.Initialize("example.com/mymod")

	if len(helper.TemplateInfo.Routes) != 0 {
		t.Error("expected Routes to be empty after Initialize")
	}
	if len(helper.TemplateInfo.ApiRoutes) != 0 {
		t.Error("expected ApiRoutes to be empty after Initialize")
	}
	if helper.TemplateInfo.GoModName != "example.com/mymod" {
		t.Errorf("expected GoModName to be 'example.com/mymod', got %q", helper.TemplateInfo.GoModName)
	}
	if helper.TemplateInfo.ImportDefault {
		t.Error("expected ImportDefault to be false after Initialize")
	}
}
