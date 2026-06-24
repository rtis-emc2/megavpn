package postgres

import "testing"

func TestIsSafeSSHAccessHost(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"bootstrap.example.com", "127.0.0.1", "[2001:db8::1]"} {
		if !isSafeSSHAccessHost(host) {
			t.Fatalf("expected ssh_host %q to be accepted", host)
		}
	}
	for _, host := range []string{"-oProxyCommand=sh", "bad..host", "[2001:db8::1", "2001:db8::zz", "host;reboot"} {
		if isSafeSSHAccessHost(host) {
			t.Fatalf("expected ssh_host %q to be rejected", host)
		}
	}
}
