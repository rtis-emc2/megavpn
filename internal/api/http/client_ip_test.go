package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresProxyHeadersByDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:49152"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	req.Header.Set("X-Real-IP", "198.51.100.21")

	s := &Server{}
	if got := s.clientIP(req); got != "192.0.2.10" {
		t.Fatalf("client ip = %q, want remote addr", got)
	}
}

func TestClientIPUsesForwardedForWhenTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:49152"
	req.Header.Set("X-Forwarded-For", "198.51.100.20, 198.51.100.30")

	s := &Server{trustProxyHeaders: true}
	if got := s.clientIP(req); got != "198.51.100.20" {
		t.Fatalf("client ip = %q, want first forwarded ip", got)
	}
}

func TestClientIPUsesRealIPWhenTrustedAndForwardedForMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:49152"
	req.Header.Set("X-Real-IP", "198.51.100.21")

	s := &Server{trustProxyHeaders: true}
	if got := s.clientIP(req); got != "198.51.100.21" {
		t.Fatalf("client ip = %q, want real ip", got)
	}
}

func TestClientIPFallsBackToRealIPWhenForwardedForInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:49152"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "198.51.100.21")

	s := &Server{trustProxyHeaders: true}
	if got := s.clientIP(req); got != "198.51.100.21" {
		t.Fatalf("client ip = %q, want real ip fallback", got)
	}
}

func TestClientIPFallsBackToRemoteAddrForInvalidProxyHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:49152"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "also-not-an-ip")

	s := &Server{trustProxyHeaders: true}
	if got := s.clientIP(req); got != "192.0.2.10" {
		t.Fatalf("client ip = %q, want remote addr fallback", got)
	}
}

func TestClientIPFallsBackWhenForwardedForStartsWithInvalidValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:49152"
	req.Header.Set("X-Forwarded-For", "not-an-ip, 198.51.100.20")

	s := &Server{trustProxyHeaders: true}
	if got := s.clientIP(req); got != "192.0.2.10" {
		t.Fatalf("client ip = %q, want remote addr fallback", got)
	}
}
