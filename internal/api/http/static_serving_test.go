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

type staticServingTestStore struct {
	Store
}

func (staticServingTestStore) Ping(context.Context) error {
	return nil
}

func TestStaticServingRoutes(t *testing.T) {
	t.Parallel()

	webRoot := t.TempDir()
	mustWriteTestFile(t, webRoot, "index.html", `<!doctype html><html><body data-console="new">new console</body></html>`)
	mustWriteTestFile(t, webRoot, filepath.Join("assets", "index.js"), `console.log("asset smoke");`)
	mustWriteTestFile(t, webRoot, filepath.Join("legacy", "index.html"), `<!doctype html><html><body data-console="legacy">legacy console</body></html>`)
	mustWriteTestFile(t, webRoot, filepath.Join("legacy", "assets", "app.js"), `window.__legacy = true;`)

	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), staticServingTestStore{}, Options{
		Version: "test",
		WebRoot: webRoot,
	})

	cases := []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
		wantMissing  string
	}{
		{name: "root returns new ui", path: "/", wantStatus: 200, wantContains: `data-console="new"`},
		{name: "clients deep link returns new ui", path: "/clients", wantStatus: 200, wantContains: `data-console="new"`},
		{name: "jobs deep link returns new ui", path: "/operations/jobs", wantStatus: 200, wantContains: `data-console="new"`},
		{name: "firewall deep link returns new ui", path: "/network-policy/firewall", wantStatus: 200, wantContains: `data-console="new"`},
		{name: "legacy returns legacy ui", path: "/legacy/", wantStatus: 200, wantContains: `data-console="legacy"`, wantMissing: `data-console="new"`},
		{name: "api ready is not shadowed", path: "/api/v1/ready", wantStatus: 200, wantContains: `"service":"megavpn-api"`, wantMissing: `data-console="new"`},
		{name: "missing new asset returns 404", path: "/assets/missing.js", wantStatus: 404, wantMissing: `data-console="new"`},
		{name: "missing backend-like download path returns 404", path: "/download/missing", wantStatus: 404, wantMissing: `data-console="new"`},
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
			body := rec.Body.String()
			if tc.wantContains != "" && !strings.Contains(body, tc.wantContains) {
				t.Fatalf("GET %s body missing %q: %s", tc.path, tc.wantContains, body)
			}
			if tc.wantMissing != "" && strings.Contains(body, tc.wantMissing) {
				t.Fatalf("GET %s body unexpectedly contains %q: %s", tc.path, tc.wantMissing, body)
			}
		})
	}
}

func TestShouldServeFrontendFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/", want: true},
		{path: "/clients", want: true},
		{path: "/operations/jobs", want: true},
		{path: "/network-policy/firewall", want: true},
		{path: "/api/v1/ready", want: false},
		{path: "/agent/jobs/next", want: false},
		{path: "/share/token", want: false},
		{path: "/subscribe/vless/token", want: false},
		{path: "/download/missing", want: false},
		{path: "/exports/report.csv", want: false},
		{path: "/assets/missing.js", want: false},
		{path: "/legacy/", want: false},
		{path: "/health", want: false},
		{path: "/ready", want: false},
		{path: "/favicon.ico", want: false},
		{path: "/unknown/file.json", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := shouldServeFrontendFallback(tc.path); got != tc.want {
				t.Fatalf("shouldServeFrontendFallback(%q) = %v, want %v", tc.path, got, tc.want)
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
