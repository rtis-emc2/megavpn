package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsUnsafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, ""}
	for _, method := range safe {
		if isUnsafeMethod(method) {
			t.Fatalf("method %q should be safe", method)
		}
	}
	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range unsafe {
		if !isUnsafeMethod(method) {
			t.Fatalf("method %q should be unsafe", method)
		}
	}
}

func TestHasCSRFHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", nil)
	if hasCSRFHeader(req) {
		t.Fatal("request without csrf header should not pass")
	}
	req.Header.Set("X-MegaVPN-CSRF", "1")
	if !hasCSRFHeader(req) {
		t.Fatal("csrf header value 1 should pass")
	}
	req.Header.Set("X-MegaVPN-CSRF", "true")
	if !hasCSRFHeader(req) {
		t.Fatal("csrf header value true should pass")
	}
	req.Header.Set("X-MegaVPN-CSRF", "false")
	if hasCSRFHeader(req) {
		t.Fatal("csrf header value false should not pass")
	}
}
