package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestParseNonce(t *testing.T) {
	proxy := NewProxyHelper()

	tests := []struct {
		name     string
		csp      string
		expected string
	}{
		{
			name:     "valid nonce",
			csp:      "script-src 'nonce-abc123'",
			expected: "abc123",
		},
		{
			name:     "no nonce",
			csp:      "script-src 'self'",
			expected: "",
		},
		{
			name:     "empty CSP",
			csp:      "",
			expected: "",
		},
		{
			name:     "nonce with multiple directives",
			csp:      "default-src 'self'; script-src 'nonce-xyz789' 'strict-dynamic'",
			expected: "xyz789",
		},
		{
			name:     "no script-src directive",
			csp:      "default-src 'self'; style-src 'unsafe-inline'",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proxy.parseNonce(tt.csp)
			if got != tt.expected {
				t.Errorf("parseNonce(%q) = %q, want %q", tt.csp, got, tt.expected)
			}
		})
	}
}

func TestInsertScriptTagIntoBody(t *testing.T) {
	proxy := NewProxyHelper()

	tests := []struct {
		name        string
		nonce       string
		body        string
		expectErr   bool
		expectInSrc string
	}{
		{
			name:        "basic HTML body",
			nonce:       "",
			body:        "<html><head></head><body><h1>Hello</h1></body></html>",
			expectErr:   false,
			expectInSrc: `src="/_gothicframework/reload/script.js"`,
		},
		{
			name:        "with nonce",
			nonce:       "abc123",
			body:        "<html><head></head><body><h1>Hello</h1></body></html>",
			expectErr:   false,
			expectInSrc: `nonce="abc123"`,
		},
		{
			name:        "minimal HTML parsed with implicit body",
			nonce:       "",
			body:        "<html><head></head></html>",
			expectErr:   false,
			expectInSrc: `src="/_gothicframework/reload/script.js"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := proxy.insertScriptTagIntoBody(tt.nonce, tt.body)
			if tt.expectErr && err == nil {
				t.Error("expected error but got nil")
				return
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.expectInSrc != "" && !strings.Contains(result, tt.expectInSrc) {
				t.Errorf("expected result to contain %q, got: %s", tt.expectInSrc, result)
			}
		})
	}
}

func TestModifyResponse_SkipNonHTML(t *testing.T) {
	proxy := NewProxyHelper()

	resp := &http.Response{
		Header:  http.Header{"Content-Type": {"application/json"}},
		Body:    io.NopCloser(strings.NewReader(`{"key":"value"}`)),
		Request: &http.Request{URL: mustParseURL("http://localhost/api")},
	}

	err := proxy.modifyResponse(resp)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Body should be unchanged
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"key":"value"}` {
		t.Errorf("expected body unchanged for non-HTML, got: %s", string(body))
	}
}

func TestModifyResponse_SkipHXRequest(t *testing.T) {
	proxy := NewProxyHelper()

	htmlContent := "<html><body>hi</body></html>"
	header := make(http.Header)
	header.Set("Content-Type", "text/html")
	header.Set("Gothic-Framework-Skip-Modify", "true")
	resp := &http.Response{
		Header:  header,
		Body:    io.NopCloser(strings.NewReader(htmlContent)),
		Request: &http.Request{URL: mustParseURL("http://localhost/")},
	}

	err := proxy.modifyResponse(resp)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if bodyStr != htmlContent {
		t.Errorf("body was modified when it shouldn't have been: got %q", bodyStr)
	}
}

func TestModifyResponse_InjectScript(t *testing.T) {
	proxy := NewProxyHelper()

	htmlBody := "<html><head></head><body><h1>Hello</h1></body></html>"
	resp := &http.Response{
		Header:  http.Header{"Content-Type": {"text/html"}},
		Body:    io.NopCloser(strings.NewReader(htmlBody)),
		Request: &http.Request{URL: mustParseURL("http://localhost/")},
	}

	err := proxy.modifyResponse(resp)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "/_gothicframework/reload/script.js") {
		t.Error("expected script tag to be injected into response")
	}
}

func TestModifyResponse_GzipEncoding(t *testing.T) {
	proxy := NewProxyHelper()

	htmlBody := "<html><head></head><body><h1>Hello</h1></body></html>"

	// Gzip encode the body
	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	gzWriter.Write([]byte(htmlBody))
	gzWriter.Close()

	resp := &http.Response{
		Header:  http.Header{"Content-Type": {"text/html"}, "Content-Encoding": {"gzip"}},
		Body:    io.NopCloser(&gzBuf),
		Request: &http.Request{URL: mustParseURL("http://localhost/")},
	}

	err := proxy.modifyResponse(resp)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Decompress the response
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	body, _ := io.ReadAll(gzReader)
	if !strings.Contains(string(body), "/_gothicframework/reload/script.js") {
		t.Error("expected script tag in gzip-encoded response")
	}
}

func TestRoundTripper_ContextCancellation(t *testing.T) {
	rt := &roundTripper{
		maxRetries:      5,
		initialDelay:    10,
		backoffExponent: 1.5,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://localhost:99999/nonexistent", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}
