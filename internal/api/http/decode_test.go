package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRejectsTrailingJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"name":"ok"}{"name":"bad"}`))
	var payload struct {
		Name string `json:"name"`
	}
	if decode(req, &payload) {
		t.Fatal("decode should reject trailing json")
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"name":"ok","unknown":true}`))
	var payload struct {
		Name string `json:"name"`
	}
	if decode(req, &payload) {
		t.Fatal("decode should reject unknown fields")
	}
}

func TestDecodeAcceptsSingleJSONDocument(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"name":"ok"}`))
	var payload struct {
		Name string `json:"name"`
	}
	if !decode(req, &payload) {
		t.Fatal("decode should accept one json document")
	}
	if payload.Name != "ok" {
		t.Fatalf("name = %q, want ok", payload.Name)
	}
}

func TestDecodeOptionalAcceptsEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	var payload struct {
		Name string `json:"name"`
	}
	if !decodeOptional(req, &payload) {
		t.Fatal("decodeOptional should accept empty body")
	}
}

func TestDecodeOptionalRejectsMalformedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(`{"name":`))
	var payload struct {
		Name string `json:"name"`
	}
	if decodeOptional(req, &payload) {
		t.Fatal("decodeOptional should reject malformed non-empty body")
	}
}
