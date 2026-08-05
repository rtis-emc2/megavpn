package http

import (
	"bufio"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type hijackableResponseWriter struct {
	nethttp.ResponseWriter
	serverConn net.Conn
	clientConn net.Conn
}

func newHijackableResponseWriter(t *testing.T) *hijackableResponseWriter {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})
	return &hijackableResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
		serverConn:     serverConn,
		clientConn:     clientConn,
	}
}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := bufio.NewReadWriter(bufio.NewReader(w.serverConn), bufio.NewWriter(w.serverConn))
	return w.serverConn, rw, nil
}

func TestLoggingResponseWriterSupportsHijack(t *testing.T) {
	t.Parallel()

	w := &lrw{ResponseWriter: newHijackableResponseWriter(t), code: 200}
	conn, rw, err := nethttp.NewResponseController(w).Hijack()
	if err != nil {
		t.Fatalf("Hijack failed through logging response writer: %v", err)
	}
	if conn == nil || rw == nil {
		t.Fatal("Hijack returned nil connection or read-writer")
	}
}

func TestSecurityHeadersAllowWebSocketConnect(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodGet, "/", nil)
	securityHeaders(true, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusNoContent)
	})).ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"connect-src", "ws://example.com", "wss://example.com"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("Content-Security-Policy = %q, want token %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("Content-Security-Policy = %q, inline styles must remain disabled", csp)
	}
	if strings.Contains(csp, "connect-src 'self' ws: wss:") {
		t.Fatalf("Content-Security-Policy = %q, websocket scheme-wide access must remain disabled", csp)
	}
}

func TestSecurityHeadersRejectHostDirectiveInjection(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/", nil)
	request.Host = "control.example; script-src *"
	securityHeaders(false, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusNoContent)
	})).ServeHTTP(recorder, request)

	csp := recorder.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, request.Host) || strings.Contains(csp, "script-src *") {
		t.Fatalf("host value was injected into Content-Security-Policy: %q", csp)
	}
	if !strings.Contains(csp, "connect-src 'self';") {
		t.Fatalf("Content-Security-Policy = %q, invalid host should leave same-origin fallback", csp)
	}
}

func TestValidCSPWebSocketHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "dns", raw: "control.example", want: "control.example"},
		{name: "dns and port", raw: "control.example:8443", want: "control.example:8443"},
		{name: "ipv4 and port", raw: "127.0.0.1:8080", want: "127.0.0.1:8080"},
		{name: "bracketed ipv6", raw: "[::1]", want: "[::1]"},
		{name: "bracketed ipv6 and port", raw: "[::1]:8443", want: "[::1]:8443"},
		{name: "directive injection", raw: "control.example; script-src *"},
		{name: "userinfo", raw: "operator@control.example"},
		{name: "invalid port", raw: "control.example:99999"},
		{name: "unbracketed ipv6", raw: "::1"},
		{name: "bracketed dns", raw: "[control.example]"},
		{name: "invalid dns label", raw: "-control.example"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := validCSPWebSocketHost(test.raw); got != test.want {
				t.Fatalf("validCSPWebSocketHost(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
