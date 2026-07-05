package backhaul

import "testing"

func TestCatalogContainsRequiredDrivers(t *testing.T) {
	t.Parallel()

	required := []string{
		DriverWireGuard,
		DriverOpenVPNUDP,
		DriverOpenVPNTCP443,
		DriverIPSecL2TP,
		DriverIKEv2,
		DriverXrayVLESSWS,
		DriverXrayVLESSGRPC,
		DriverXrayReality,
		DriverXrayTUNVLESS,
	}
	for _, driver := range required {
		if _, ok := Definition(driver); !ok {
			t.Fatalf("driver %s missing from catalog", driver)
		}
	}
}

func TestValidateTunnelCIDR(t *testing.T) {
	t.Parallel()

	if err := ValidateTunnelCIDR("10.240.1.0/30"); err != nil {
		t.Fatalf("valid /30 rejected: %v", err)
	}
	if err := ValidateTunnelCIDR("2001:db8::/64"); err == nil {
		t.Fatal("IPv6 tunnel CIDR must be rejected for current backhaul scope")
	}
	if err := ValidateTunnelCIDR("10.240.1.0/32"); err == nil {
		t.Fatal("/32 tunnel CIDR must be rejected")
	}
}
