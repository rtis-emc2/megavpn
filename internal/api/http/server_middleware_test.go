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
	for _, want := range []string{"connect-src", "ws:", "wss:"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("Content-Security-Policy = %q, want token %q", csp, want)
		}
	}
}
