package postgres

import "testing"

func TestNextAvailableIPv4SubnetStartsAtConfiguredCIDR(t *testing.T) {
	got, err := nextAvailableIPv4Subnet("172.16.0.0/12", "172.16.112.0/24", 24, nil)
	if err != nil {
		t.Fatalf("nextAvailableIPv4Subnet() error = %v", err)
	}
	if got.String() != "172.16.112.0/24" {
		t.Fatalf("next subnet = %s, want 172.16.112.0/24", got.String())
	}
}

func TestNextAvailableIPv4SubnetSkipsUsed(t *testing.T) {
	got, err := nextAvailableIPv4Subnet("172.16.0.0/12", "172.16.112.0/24", 24, map[string]bool{
		"172.16.112.0/24": true,
		"172.16.113.0/24": true,
	})
	if err != nil {
		t.Fatalf("nextAvailableIPv4Subnet() error = %v", err)
	}
	if got.String() != "172.16.114.0/24" {
		t.Fatalf("next subnet = %s, want 172.16.114.0/24", got.String())
	}
}

func TestNextAvailableIPv4SubnetReportsExhaustion(t *testing.T) {
	_, err := nextAvailableIPv4Subnet("10.0.0.0/30", "10.0.0.0/30", 30, map[string]bool{
		"10.0.0.0/30": true,
	})
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
}

func TestL2TPPoolFromPrefix(t *testing.T) {
	localIP, start, end, err := l2tpPoolFromPrefix("172.16.120.0/24")
	if err != nil {
		t.Fatalf("l2tpPoolFromPrefix() error = %v", err)
	}
	if localIP != "172.16.120.1" || start != "172.16.120.10" || end != "172.16.120.200" {
		t.Fatalf("l2tp pool = %s %s-%s", localIP, start, end)
	}
}
