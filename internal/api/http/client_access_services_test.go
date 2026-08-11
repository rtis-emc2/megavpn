package http

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestClientAccessServiceCatalog(t *testing.T) {
	services := clientAccessServiceCatalog()
	if len(services) == 0 {
		t.Fatal("client access service catalog is empty")
	}
	byCode := map[string]int{}
	for _, service := range services {
		if service.ServiceCode == "" {
			t.Fatalf("service code is required: %#v", service)
		}
		byCode[service.ServiceCode]++
		if byCode[service.ServiceCode] > 1 {
			t.Fatalf("duplicate service code %q", service.ServiceCode)
		}
		if service.DisplayName == "" || service.Description == "" || service.Category == "" || service.Status == "" {
			t.Fatalf("service %q is missing operator metadata: %#v", service.ServiceCode, service)
		}
	}
	expectedReady := []string{
		"vless",
		"openvpn",
		"wireguard",
		"l2tp",
		"http_proxy",
		"shadowsocks",
		"mtproto",
	}
	for _, code := range expectedReady {
		service := findClientAccessServiceForTest(services, code)
		if service == nil {
			t.Fatalf("%s must be visible in client access service catalog", code)
		}
		if !service.Implemented || !service.SupportsGroups || !service.SupportsMembership || !service.SupportsMaterialization || service.Status != "active" {
			t.Fatalf("%s catalog flags = %#v, want active group materialization", code, service)
		}
	}
	socks := findClientAccessServiceForTest(services, "socks_proxy")
	if socks == nil {
		t.Fatal("socks_proxy must be visible in client access service catalog")
	}
	if socks.Implemented || socks.SupportsGroups || socks.SupportsMembership || socks.SupportsMaterialization || socks.Status == "active" {
		t.Fatalf("socks_proxy catalog flags = %#v, want planned catalog entry", socks)
	}
	if nginx := findClientAccessServiceForTest(services, "nginx"); nginx != nil {
		t.Fatalf("nginx is an edge runtime, not a client access service: %#v", nginx)
	}
}

func findClientAccessServiceForTest(services []domain.ClientAccessService, code string) *domain.ClientAccessService {
	for idx := range services {
		if services[idx].ServiceCode == code {
			return &services[idx]
		}
	}
	return nil
}
