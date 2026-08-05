package externalegress

import "testing"

func TestAvailableCatalogContainsOnlyRuntimeReadyProtocols(t *testing.T) {
	t.Parallel()

	catalog := AvailableCatalog()
	if len(catalog) == 0 {
		t.Fatal("available catalog is empty")
	}
	for _, item := range catalog {
		if item.RuntimeSupport != RuntimeReady {
			t.Fatalf("available catalog contains %s protocol %q", item.RuntimeSupport, item.Code)
		}
	}
	for _, code := range []string{"openvpn", "wireguard", "shadowsocks", "vless", "l2tp_ipsec"} {
		found := false
		for _, item := range catalog {
			found = found || item.Code == code
		}
		if !found {
			t.Fatalf("runtime-ready protocol %q is missing", code)
		}
	}
}
