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

func TestNormalizePEMBlockWrapsPEMForInlineOpenVPN(t *testing.T) {
	pem := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"
	got := normalizePEMBlock("ca", pem)
	for _, want := range []string{
		"<ca>\n",
		"-----BEGIN CERTIFICATE-----",
		"-----END CERTIFICATE-----",
		"\n</ca>\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized PEM missing %q:\n%s", want, got)
		}
	}
}

func TestNormalizePEMBlockPreservesExistingInlineBlock(t *testing.T) {
	inline := "<cert>\n-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n</cert>"
	got := normalizePEMBlock("cert", inline)
	if got != inline+"\n" {
		t.Fatalf("inline block = %q, want preserved with trailing newline", got)
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

func TestBuildXrayArtifactsIncludesVLESSClientProfile(t *testing.T) {
	record := domain.ProvisioningAccess{
		Access: domain.ServiceAccess{
			ID: "access-xray",
			Metadata: map[string]any{
				"xray_uuid": "3c917b7e-4cf7-48f9-95bf-0f0a35f82b93",
				"inbound_service": map[string]any{
					"service_code":  "xray-core",
					"service_label": "Xray VLESS",
					"endpoint":      "vpn.example.test:443",
				},
				"vless_group": "restricted",
			},
		},
		Client: domain.Client{ID: "client-1", Username: "client-one", Email: "client@example.test"},
		Instance: domain.Instance{
			ID:           "instance-1",
			Name:         "edge-vless",
			Slug:         "edge-vless",
			ServiceCode:  "xray-core",
			EndpointHost: "vpn.example.test",
			EndpointPort: 443,
			Spec: map[string]any{
				"security":           "reality",
				"network":            "tcp",
				"server_name":        "vpn.example.test",
				"fingerprint":        "chrome",
				"reality_public_key": "public-key-value",
				"short_id":           "0123456789abcdef",
				"flow":               "xtls-rprx-vision",
			},
		},
	}

	files, err := buildXrayArtifacts(record)
	if err != nil {
		t.Fatalf("build xray artifacts: %v", err)
	}
	if len(files) != 1 || files[0].ArtifactType != "vless_url" {
		t.Fatalf("files = %#v, want one vless_url artifact", files)
	}
	body := string(files[0].Content)
	for _, want := range []string{
		"VLESS client access",
		"vless://3c917b7e-4cf7-48f9-95bf-0f0a35f82b93@vpn.example.test:443?",
		"Reality public key: public-key-value",
		"Client JSON:",
		`"protocol": "vless"`,
		`"inbound_service"`,
		"Access group: restricted",
		`"outbound_group": "restricted"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("artifact body missing %q:\n%s", want, body)
		}
	}
}

func TestBuildXrayArtifactsUsesCamouflagePublicEndpoint(t *testing.T) {
	record := domain.ProvisioningAccess{
		Access: domain.ServiceAccess{
			ID: "access-xray-camouflage",
			Metadata: map[string]any{
				"xray_uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			},
		},
		Client: domain.Client{ID: "client-1", Username: "client-one"},
		Instance: domain.Instance{
			ID:           "instance-1",
			Name:         "edge-xray-http-xray-ws",
			Slug:         "edge-xray-http-xray-ws",
			ServiceCode:  "xray-core",
			EndpointHost: "enter.example.test",
			EndpointPort: 7080,
			Spec: map[string]any{
				"security":           "none",
				"network":            "ws",
				"path":               "/assets/rtis-sync",
				"public_security":    "tls",
				"public_network":     "ws",
				"public_path":        "/assets/rtis-sync",
				"public_host_header": "enter.example.test",
				"public_port":        443,
			},
		},
	}

	files, err := buildXrayArtifacts(record)
	if err != nil {
		t.Fatalf("build xray artifacts: %v", err)
	}
	if len(files) != 1 || files[0].ArtifactType != "vless_url" {
		t.Fatalf("files = %#v, want one vless_url", files)
	}
	body := string(files[0].Content)
	for _, want := range []string{
		"vless://aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa@enter.example.test:443?",
		"security=tls",
		"type=ws",
		"path=%2Fassets%2Frtis-sync",
		"host=enter.example.test",
		"Endpoint: enter.example.test:443",
		`"path": "/assets/rtis-sync"`,
		`"host_header": "enter.example.test"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("camouflage artifact body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "@enter.example.test:7080?") || strings.Contains(body, "Endpoint: enter.example.test:7080") {
		t.Fatalf("camouflage artifact leaked backend endpoint:\n%s", body)
	}
}
