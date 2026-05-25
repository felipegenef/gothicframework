package gothicComponents

import (
	"bytes"
	"context"
	"strings"
	"testing"

	routes "github.com/felipegenef/gothicframework/pkg/helpers/routes"
)

// renderStatefulComponent renders the StatefulComponent templ component to a
// string so we can assert on the emitted HTML without a browser.
func renderStatefulComponent(t *testing.T, path string, method routes.HttpMethod, vals string, hasVals bool) string {
	t.Helper()
	var buf bytes.Buffer
	if err := StatefulComponent(path, method, vals, hasVals).Render(context.Background(), &buf); err != nil {
		t.Fatalf("StatefulComponent.Render: %v", err)
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// Q1: hx-vals absent when no vals are provided
// ---------------------------------------------------------------------------

// TestStatefulComponent_NoVals_OmitsHxVals asserts that when StatefulComponentOf
// is called without any vals argument the rendered HTML must NOT contain an
// hx-vals attribute at all. Previously the component always emitted hx-vals="{}"
// even for the empty-map case.
func TestStatefulComponent_NoVals_OmitsHxVals(t *testing.T) {
	config := &routes.RouteConfig[any]{
		Path:       "/ping",
		HttpMethod: routes.GET,
	}
	// No vals argument — the variadic is empty.
	comp := StatefulComponentOf(config)

	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Fatalf("StatefulComponentOf (no vals): Render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "hx-vals") {
		t.Errorf("expected hx-vals to be absent when no vals are passed, got: %s", html)
	}
}

// TestStatefulComponent_NilVals_OmitsHxVals confirms that an explicit nil
// StatefulComponentData also suppresses hx-vals.
func TestStatefulComponent_NilVals_OmitsHxVals(t *testing.T) {
	config := &routes.RouteConfig[any]{
		Path:       "/ping",
		HttpMethod: routes.POST,
	}
	comp := StatefulComponentOf(config, nil)

	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Fatalf("StatefulComponentOf (nil vals): Render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "hx-vals") {
		t.Errorf("expected hx-vals to be absent for nil vals, got: %s", html)
	}
}

// TestStatefulComponent_EmptyMapVals_OmitsHxVals confirms that an empty (but
// non-nil) StatefulComponentData also suppresses hx-vals, because an empty map
// carries no information and hx-vals={} would be noise.
func TestStatefulComponent_EmptyMapVals_OmitsHxVals(t *testing.T) {
	config := &routes.RouteConfig[any]{
		Path:       "/ping",
		HttpMethod: routes.GET,
	}
	comp := StatefulComponentOf(config, StatefulComponentData{})

	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Fatalf("StatefulComponentOf (empty map): Render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "hx-vals") {
		t.Errorf("expected hx-vals to be absent for empty map vals, got: %s", html)
	}
}

// ---------------------------------------------------------------------------
// Q2: hx-vals present and contains the correct JSON when vals are provided
// ---------------------------------------------------------------------------

// TestStatefulComponent_WithVals_EmitsHxVals asserts that when a non-empty
// StatefulComponentData is passed, hx-vals is emitted and its value is the
// encoded JSON string (matching getAttribute('hx-vals') in a browser).
func TestStatefulComponent_WithVals_EmitsHxVals(t *testing.T) {
	config := &routes.RouteConfig[any]{
		Path:       "/counter",
		HttpMethod: routes.GET,
	}
	comp := StatefulComponentOf(config, StatefulComponentData{"id": "42"})

	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Fatalf("StatefulComponentOf (with vals): Render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "hx-vals") {
		t.Errorf("expected hx-vals to be present when vals are provided, got: %s", html)
	}
	// templ HTML-escapes attribute values: " becomes &#34;. We look for the
	// HTML-entity-encoded form of the JSON key, which is what getAttribute('hx-vals')
	// returns after the browser decodes it.
	if !strings.Contains(html, "&#34;id&#34;") {
		t.Errorf("expected hx-vals to contain the key \"id\" (HTML-escaped as &#34;id&#34;), got: %s", html)
	}
}

// ---------------------------------------------------------------------------
// Q3: multi-field vals (id + label) round-trip through the rendered attribute
// ---------------------------------------------------------------------------

// DataEchoProps mirrors the props that a POST middleware would echo back when
// both "id" and "label" form values are included in the HTMX request (via
// hx-vals). The test verifies both fields survive the marshal → HTML emit path.
type DataEchoProps struct {
	ID    string
	Label string
}

// TestStatefulComponent_MultiFieldVals_BothFieldsPresent verifies that when
// StatefulComponentData carries two fields (id and label), the emitted hx-vals
// attribute encodes both. This corresponds to a POST route whose middleware reads
// r.FormValue("id") and r.FormValue("label") — both must round-trip.
func TestStatefulComponent_MultiFieldVals_BothFieldsPresent(t *testing.T) {
	config := &routes.RouteConfig[any]{
		Path:       "/item",
		HttpMethod: routes.POST,
	}
	comp := StatefulComponentOf(config, StatefulComponentData{
		"id":    "123",
		"label": "hello world",
	})

	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Fatalf("StatefulComponentOf (multi-field): Render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "hx-vals") {
		t.Errorf("expected hx-vals attribute to be present, got: %s", html)
	}
	// templ HTML-escapes attribute values: " becomes &#34;. Both keys must
	// appear in their HTML-entity-encoded form (matching getAttribute('hx-vals')
	// after browser decode).
	for _, key := range []string{"&#34;id&#34;", "&#34;label&#34;"} {
		if !strings.Contains(html, key) {
			t.Errorf("expected hx-vals to contain key %s (HTML-escaped), got: %s", key, html)
		}
	}
	// Both values must appear too.
	for _, val := range []string{"123", "hello world"} {
		if !strings.Contains(html, val) {
			t.Errorf("expected hx-vals to contain value %q, got: %s", val, html)
		}
	}
}

// ---------------------------------------------------------------------------
// Direct StatefulComponent lower-level tests (hasVals flag)
// ---------------------------------------------------------------------------

// TestStatefulComponent_HasValsFalse_AllMethods checks that hasVals=false
// suppresses hx-vals on every HTTP method branch.
func TestStatefulComponent_HasValsFalse_AllMethods(t *testing.T) {
	methods := []routes.HttpMethod{
		routes.GET, routes.POST, routes.PUT, routes.PATCH, routes.DELETE,
	}
	for _, m := range methods {
		html := renderStatefulComponent(t, "/x", m, "", false)
		if strings.Contains(html, "hx-vals") {
			t.Errorf("method %d: expected no hx-vals with hasVals=false, got: %s", m, html)
		}
	}
}

// TestStatefulComponent_HasValsTrue_AllMethods checks that hasVals=true
// emits hx-vals on every HTTP method branch and carries the supplied value.
func TestStatefulComponent_HasValsTrue_AllMethods(t *testing.T) {
	methods := []routes.HttpMethod{
		routes.GET, routes.POST, routes.PUT, routes.PATCH, routes.DELETE,
	}
	for _, m := range methods {
		html := renderStatefulComponent(t, "/x", m, `{"k":"v"}`, true)
		if !strings.Contains(html, "hx-vals") {
			t.Errorf("method %d: expected hx-vals with hasVals=true, got: %s", m, html)
		}
	}
}

// ---------------------------------------------------------------------------
// toStatefulComponentData unit tests
// ---------------------------------------------------------------------------

func TestToStatefulComponentData_NilReturnsEmpty(t *testing.T) {
	_, hasVals := toStatefulComponentData(nil)
	if hasVals {
		t.Error("expected hasVals=false for nil input")
	}
}

func TestToStatefulComponentData_EmptyMapReturnsEmpty(t *testing.T) {
	_, hasVals := toStatefulComponentData(StatefulComponentData{})
	if hasVals {
		t.Error("expected hasVals=false for empty map")
	}
}

func TestToStatefulComponentData_NonEmptyReturnsJSON(t *testing.T) {
	s, hasVals := toStatefulComponentData(StatefulComponentData{"x": 1})
	if !hasVals {
		t.Error("expected hasVals=true for non-empty map")
	}
	if !strings.Contains(s, `"x"`) {
		t.Errorf("expected JSON to contain key x, got: %s", s)
	}
}

// TestToStatefulComponentData_MultiField verifies both ID and Label fields are
// present in the marshalled JSON — this is the Q3 multi-field echo scenario.
func TestToStatefulComponentData_MultiField(t *testing.T) {
	props := DataEchoProps{ID: "abc", Label: "my label"}
	data := StatefulComponentData{
		"id":    props.ID,
		"label": props.Label,
	}
	s, hasVals := toStatefulComponentData(data)
	if !hasVals {
		t.Fatal("expected hasVals=true")
	}
	for _, want := range []string{`"id"`, `"label"`, "abc", "my label"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected JSON to contain %q, got: %s", want, s)
		}
	}
}
