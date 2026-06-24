package postgres

import "testing"

func TestValidateControlPlaneServerNameRejectsDirectiveInjection(t *testing.T) {
	t.Parallel()

	if err := validateControlPlaneServerName("control.example.com; root /"); err == nil {
		t.Fatal("expected directive injection server_name to be rejected")
	}
}

func TestValidateControlPlaneServerNameRejectsMalformedIPLiteral(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"[2001:db8::1", "2001:db8::zz"} {
		if err := validateControlPlaneServerName(name); err == nil {
			t.Fatalf("expected malformed IP literal %q to be rejected", name)
		}
	}
}

func TestValidateControlPlaneServerNameAllowsDNSAndWildcard(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"control.example.com", "*.control.example.com", "_", "127.0.0.1", "[2001:db8::1]"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateControlPlaneServerName(name); err != nil {
				t.Fatalf("validateControlPlaneServerName(%q) = %v", name, err)
			}
		})
	}
}
