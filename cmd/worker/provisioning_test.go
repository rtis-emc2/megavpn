package main

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestSelectArtifactFilesForSpecificType(t *testing.T) {
	files := []generatedArtifactFile{
		{ArtifactType: "wg_conf", Filename: "client.conf"},
		{ArtifactType: "ovpn", Filename: "client.ovpn"},
	}

	selected := selectArtifactFiles(files, "wg_conf")
	if len(selected) != 1 || selected[0].ArtifactType != "wg_conf" {
		t.Fatalf("selected = %#v, want only wg_conf", selected)
	}
}

func TestSelectArtifactFilesForZipBundleReturnsBaseFilesForArchiveBuild(t *testing.T) {
	files := []generatedArtifactFile{
		{ArtifactType: "wg_conf", Filename: "client.conf"},
		{ArtifactType: "ovpn", Filename: "client.ovpn"},
	}

	selected := selectArtifactFiles(files, "zip_bundle")
	if len(selected) != len(files) {
		t.Fatalf("selected = %#v, want all base files for zip build", selected)
	}
}

func TestBuildShadowsocksArtifactsUsesManagedAccountFallback(t *testing.T) {
	record := domain.ProvisioningAccess{
		Access: domain.ServiceAccess{ID: "access-1", Metadata: map[string]any{}},
		Client: domain.Client{Username: "client-one"},
		Instance: domain.Instance{
			Name:         "edge-ss",
			Slug:         "edge-ss",
			EndpointHost: "vpn.example.test",
			Spec: map[string]any{
				"method": "chacha20-ietf-poly1305",
				"managed_accounts": []any{
					map[string]any{
						"service_access_id": "access-1",
						"server_port":       8391,
						"password":          "strong-password-value",
					},
				},
			},
		},
	}

	files, err := buildShadowsocksArtifacts(record)
	if err != nil {
		t.Fatalf("build shadowsocks artifacts: %v", err)
	}
	if len(files) != 1 || files[0].ArtifactType != "ss_url" {
		t.Fatalf("files = %#v, want one ss_url artifact", files)
	}
	body := string(files[0].Content)
	if !strings.Contains(body, "@vpn.example.test:8391") {
		t.Fatalf("artifact body = %q, want managed account endpoint", body)
	}
}
