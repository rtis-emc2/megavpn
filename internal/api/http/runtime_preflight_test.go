package http

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimePreflightStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		checks []runtimePreflightCheck
		want   string
	}{
		{
			name: "ready when all checks pass",
			checks: []runtimePreflightCheck{
				{Status: "ok"},
				{Status: "ok"},
			},
			want: "ready",
		},
		{
			name: "degraded when any check warns",
			checks: []runtimePreflightCheck{
				{Status: "ok"},
				{Status: "warning"},
			},
			want: "degraded",
		},
		{
			name: "blocked when any check fails",
			checks: []runtimePreflightCheck{
				{Status: "warning"},
				{Status: "failed"},
			},
			want: "blocked",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runtimePreflightStatus(tc.checks); got != tc.want {
				t.Fatalf("runtimePreflightStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRuntimePreflightIsReady(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		checks []runtimePreflightCheck
		want   bool
	}{
		{
			name: "ready when all checks pass",
			checks: []runtimePreflightCheck{
				{Status: "ok"},
				{Status: "ok"},
			},
			want: true,
		},
		{
			name: "not ready when any check warns",
			checks: []runtimePreflightCheck{
				{Status: "ok"},
				{Status: "warning"},
			},
			want: false,
		},
		{
			name: "not ready when any check fails",
			checks: []runtimePreflightCheck{
				{Status: "failed"},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runtimePreflightIsReady(tc.checks); got != tc.want {
				t.Fatalf("runtimePreflightIsReady() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsLocalControlPlaneHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "::1", want: true},
		{host: "10.10.20.30", want: true},
		{host: "control.example.com", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			if got := isLocalControlPlaneHost(tc.host); got != tc.want {
				t.Fatalf("isLocalControlPlaneHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestIsLoopbackListenAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:8080", want: true},
		{addr: "[::1]:8080", want: true},
		{addr: "localhost:8080", want: true},
		{addr: ":8080", want: false},
		{addr: "0.0.0.0:8080", want: false},
		{addr: "10.0.0.10:8080", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()
			if got := isLoopbackListenAddress(tc.addr); got != tc.want {
				t.Fatalf("isLoopbackListenAddress(%q) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestProductionPreflightSeverity(t *testing.T) {
	t.Parallel()

	t.Run("public base url http is failed in production", func(t *testing.T) {
		t.Parallel()
		check := (&Server{publicBaseURL: "http://control.example.com", productionMode: true}).publicBaseURLPreflightCheck()
		if check.Status != "failed" {
			t.Fatalf("status = %q, want failed: %#v", check.Status, check)
		}
	})

	t.Run("public base url localhost is failed in production", func(t *testing.T) {
		t.Parallel()
		check := (&Server{publicBaseURL: "https://127.0.0.1:8443", productionMode: true}).publicBaseURLPreflightCheck()
		if check.Status != "failed" {
			t.Fatalf("status = %q, want failed: %#v", check.Status, check)
		}
	})

	t.Run("trusted proxy headers are accepted behind loopback in production", func(t *testing.T) {
		t.Parallel()
		check := (&Server{listenAddr: "127.0.0.1:8080", trustProxyHeaders: true, productionMode: true}).proxyHeadersPreflightCheck()
		if check.Status != "ok" {
			t.Fatalf("status = %q, want ok: %#v", check.Status, check)
		}
	})

	t.Run("trusted proxy headers fail on wildcard listener in production", func(t *testing.T) {
		t.Parallel()
		check := (&Server{listenAddr: "0.0.0.0:8080", trustProxyHeaders: true, productionMode: true}).proxyHeadersPreflightCheck()
		if check.Status != "failed" {
			t.Fatalf("status = %q, want failed: %#v", check.Status, check)
		}
	})

	t.Run("external tls edge is accepted when public url and listener are production-shaped", func(t *testing.T) {
		t.Parallel()
		server := &Server{
			publicBaseURL:     "https://control.example.com:58765",
			listenAddr:        "127.0.0.1:8080",
			trustProxyHeaders: true,
			productionMode:    true,
		}
		if !server.externalControlPlaneTLSAssumed() {
			t.Fatal("externalControlPlaneTLSAssumed() = false, want true")
		}
	})
}

func TestArtifactStoragePreflightCheck(t *testing.T) {
	t.Parallel()

	t.Run("existing directory is ok", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		check := (&Server{artifactRoot: dir}).artifactStoragePreflightCheck()
		if check.Status != "ok" {
			t.Fatalf("status = %q, want ok: %#v", check.Status, check)
		}
	})

	t.Run("missing root with existing parent is warning", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "artifacts")
		check := (&Server{artifactRoot: root}).artifactStoragePreflightCheck()
		if check.Status != "warning" {
			t.Fatalf("status = %q, want warning: %#v", check.Status, check)
		}
	})

	t.Run("missing root with existing parent is failed in production", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "artifacts")
		check := (&Server{artifactRoot: root, productionMode: true}).artifactStoragePreflightCheck()
		if check.Status != "failed" {
			t.Fatalf("status = %q, want failed: %#v", check.Status, check)
		}
	})

	t.Run("file root is failed", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "artifacts")
		if err := os.WriteFile(root, []byte("not a directory\n"), 0o600); err != nil {
			t.Fatalf("write file root: %v", err)
		}
		check := (&Server{artifactRoot: root}).artifactStoragePreflightCheck()
		if check.Status != "failed" {
			t.Fatalf("status = %q, want failed: %#v", check.Status, check)
		}
	})
}
