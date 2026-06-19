package http

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCookieSecureAutoAllowsPlainHTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	s := &Server{sessionCookieSecure: true}

	if s.sessionCookieSecureForRequest(req) {
		t.Fatal("auto secure cookie must not set Secure on plain HTTP operator requests")
	}
}

func TestSessionCookieSecureAutoUsesTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.TLS = &tls.ConnectionState{}
	s := &Server{sessionCookieSecure: true}

	if !s.sessionCookieSecureForRequest(req) {
		t.Fatal("auto secure cookie should set Secure on TLS requests")
	}
}

func TestSessionCookieSecureAutoUsesTrustedForwardedProto(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	s := &Server{sessionCookieSecure: true, trustProxyHeaders: true}

	if !s.sessionCookieSecureForRequest(req) {
		t.Fatal("auto secure cookie should trust forwarded https when proxy headers are trusted")
	}
}

func TestSessionCookieSecureExplicitAlwaysSecure(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	s := &Server{sessionCookieSecure: true, sessionCookieSecureSet: true}

	if !s.sessionCookieSecureForRequest(req) {
		t.Fatal("explicit secure cookie setting should be honored")
	}
}
