package http

import (
	"context"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticServingTestStore struct{ Store }

func (staticServingTestStore) Ping(context.Context) error { return nil }

func TestStaticServingRoutes(t *testing.T) {
	t.Parallel()

	webRoot := t.TempDir()
	mustWriteTestFile(t, webRoot, "index.html", `<!doctype html><html><body data-console="react">console</body></html>`)
	mustWriteTestFile(t, webRoot, filepath.Join("assets", "index-deadbeef.js"), `window.__console = true;`)

	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), staticServingTestStore{}, Options{
		Version: "test",
		WebRoot: webRoot,
	})

	cases := []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
		wantCache    string
	}{
		{name: "root", path: "/", wantStatus: 200, wantContains: `data-console="react"`, wantCache: "no-store"},
		{name: "deep link", path: "/infrastructure/external-egress", wantStatus: 200, wantContains: `data-console="react"`, wantCache: "no-store"},
		{name: "hashed asset", path: "/assets/index-deadbeef.js", wantStatus: 200, wantContains: `window.__console`, wantCache: "immutable"},
		{name: "api is not shadowed", path: "/api/v1/ready", wantStatus: 200, wantContains: `"service":"megavpn-api"`},
		{name: "missing asset", path: "/assets/missing.js", wantStatus: 404},
		{name: "missing api", path: "/api/v1/does-not-exist", wantStatus: 404},
		{name: "missing download", path: "/download/missing", wantStatus: 404},
		{name: "legacy route removed", path: "/legacy/", wantStatus: 404},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(nethttp.MethodGet, tc.path, nil)
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("GET %s status = %d, want %d; body: %s", tc.path, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantContains != "" && !strings.Contains(rec.Body.String(), tc.wantContains) {
				t.Fatalf("GET %s body missing %q: %s", tc.path, tc.wantContains, rec.Body.String())
			}
			if tc.wantCache != "" && !strings.Contains(rec.Header().Get("Cache-Control"), tc.wantCache) {
				t.Fatalf("GET %s Cache-Control = %q, want token %q", tc.path, rec.Header().Get("Cache-Control"), tc.wantCache)
			}
		})
	}
}

func TestShouldServeFrontendFallback(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"/":                               true,
		"/clients":                        true,
		"/infrastructure/external-egress": true,
		"/legacy/":                        false,
		"/api/v1/ready":                   false,
		"/agent/jobs/next":                false,
		"/share/token":                    false,
		"/subscribe/vless/token":          false,
		"/download/missing":               false,
		"/exports/report.csv":             false,
		"/assets/missing.js":              false,
		"/health":                         false,
		"/favicon.ico":                    false,
		"/unknown/file.json":              false,
	}
	for path, want := range cases {
		path, want := path, want
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if got := shouldServeFrontendFallback(path); got != want {
				t.Fatalf("shouldServeFrontendFallback(%q) = %v, want %v", path, got, want)
			}
		})
	}
}

func mustWriteTestFile(t *testing.T, root, relPath, contents string) {
	t.Helper()
	absPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", absPath, err)
	}
}
