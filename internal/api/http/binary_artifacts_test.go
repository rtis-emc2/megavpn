package http

import (
	"net"
	"net/url"
	"testing"
)

func TestValidateRemoteArtifactURLRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()

	tests := []string{
		"http://example.com/xray",
		"https://localhost/xray",
		"https://127.0.0.1/xray",
		"https://10.0.0.10/xray",
		"https://172.16.0.10/xray",
		"https://192.168.1.10/xray",
		"https://[::1]/xray",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse URL: %v", err)
			}
			if err := validateRemoteArtifactURL(u); err == nil {
				t.Fatal("validateRemoteArtifactURL accepted unsafe URL")
			}
		})
	}
}

func TestValidateRemoteArtifactURLAllowsHTTPSDNSName(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://downloads.example.com/runtime/xray")
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if err := validateRemoteArtifactURL(u); err != nil {
		t.Fatalf("validateRemoteArtifactURL rejected HTTPS DNS URL: %v", err)
	}
}

func TestSafeRemoteArtifactIP(t *testing.T) {
	t.Parallel()

	if !isSafeRemoteArtifactIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1"} {
		if isSafeRemoteArtifactIP(net.ParseIP(raw)) {
			t.Fatalf("unsafe IP accepted: %s", raw)
		}
	}
}

func TestRemoteArtifactFilename(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://downloads.example.com/releases/xray%20linux")
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if got := remoteArtifactFilename(u, `attachment; filename="xray"`); got != "xray" {
		t.Fatalf("filename from content-disposition = %q", got)
	}
	if got := remoteArtifactFilename(u, ""); got != "xray linux" {
		t.Fatalf("filename from URL = %q", got)
	}
}
