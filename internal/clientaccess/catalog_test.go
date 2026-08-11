package clientaccess

import "testing"

func TestCatalogMaterializationReadiness(t *testing.T) {
	ready := []string{"vless", "openvpn", "wireguard", "l2tp", "http_proxy", "shadowsocks", "mtproto"}
	for _, serviceCode := range ready {
		if !MaterializationReady(serviceCode) {
			t.Fatalf("service %q must support group materialization", serviceCode)
		}
	}
	if MaterializationReady("socks_proxy") {
		t.Fatal("socks_proxy must remain unavailable until its runtime materializer exists")
	}
}

func TestCatalogReturnsIndependentCapabilityData(t *testing.T) {
	first := Catalog()
	if len(first) == 0 {
		t.Fatal("catalog is empty")
	}
	first[0].RuntimeServiceCodes[0] = "mutated"
	first[0].PolicyCapabilities["modes"].([]string)[0] = "mutated"
	first[0].PolicyCapabilities["routing"].(map[string]bool)["egress_node"] = false

	second := Catalog()
	if second[0].RuntimeServiceCodes[0] != "xray-core" {
		t.Fatalf("runtime service code leaked mutation: %#v", second[0].RuntimeServiceCodes)
	}
	if second[0].PolicyCapabilities["modes"].([]string)[0] != "instance_default" {
		t.Fatalf("policy modes leaked mutation: %#v", second[0].PolicyCapabilities)
	}
	if !second[0].PolicyCapabilities["routing"].(map[string]bool)["egress_node"] {
		t.Fatalf("routing capability leaked mutation: %#v", second[0].PolicyCapabilities)
	}
}
