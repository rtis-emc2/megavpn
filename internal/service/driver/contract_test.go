package driver

import "testing"

func TestNormalizeCodeAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"xray":              XrayCore,
		"xray_core":         XrayCore,
		"wg-quick":          WireGuard,
		"squid":             HTTPProxy,
		"shadowsocks-libev": Shadowsocks,
		"strongswan":        IPSec,
		"l2tpd":             XL2TPD,
	}
	for input, want := range cases {
		if got := NormalizeCode(input); got != want {
			t.Fatalf("NormalizeCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestContractDefaults(t *testing.T) {
	t.Parallel()

	if got := DefaultSystemdUnit(OpenVPN, "edge"); got != "openvpn-server@edge" {
		t.Fatalf("openvpn unit = %q", got)
	}
	if got := DefaultConfigPath(WireGuard, "wg-edge"); got != "/etc/wireguard/wg-edge.conf" {
		t.Fatalf("wireguard config path = %q", got)
	}
	if got := DefaultConfigMode(WireGuard); got != "0600" {
		t.Fatalf("wireguard config mode = %q", got)
	}
	if !RequiresIPForwarding(IPSec) {
		t.Fatal("ipsec must require ip forwarding")
	}
	if RequiresIPForwarding(Nginx) {
		t.Fatal("nginx must not require ip forwarding")
	}
}

func TestArtifactTypeAllowlist(t *testing.T) {
	t.Parallel()

	if !IsSupportedArtifactType(ArtifactWireGuardConfig) {
		t.Fatal("wg_conf should be supported")
	}
	if IsSupportedArtifactType("raw_secret_dump") {
		t.Fatal("raw_secret_dump must not be supported")
	}
}

func TestDriverOperationContract(t *testing.T) {
	t.Parallel()

	for _, code := range []string{OpenVPN, XrayCore, WireGuard, IPSec, HTTPProxy, Shadowsocks, MTProto, Nginx} {
		contract, ok := ContractFor(code)
		if !ok {
			t.Fatalf("ContractFor(%s) not found", code)
		}
		if len(contract.Operations) == 0 {
			t.Fatalf("ContractFor(%s) has no operations", code)
		}
		if !SupportsOperation(code, OperationApply) {
			t.Fatalf("%s must support apply", code)
		}
		if !SupportsOperation(code, OperationRestart) {
			t.Fatalf("%s must support restart", code)
		}
		op, ok := OperationFor(code, OperationApply)
		if !ok {
			t.Fatalf("OperationFor(%s, apply) not found", code)
		}
		if op.JobType != "instance.apply" || !op.RequiresSpec || !op.MutatesConfig || !op.MutatesRuntime {
			t.Fatalf("%s apply operation = %#v, want job-backed mutating apply", code, op)
		}
	}
}

func TestDriverHealthCheckContract(t *testing.T) {
	t.Parallel()

	for _, code := range []string{OpenVPN, XrayCore, WireGuard, IPSec, HTTPProxy, Shadowsocks, MTProto, Nginx} {
		contract, ok := ContractFor(code)
		if !ok {
			t.Fatalf("ContractFor(%s) not found", code)
		}
		if len(contract.HealthChecks) == 0 {
			t.Fatalf("ContractFor(%s) has no health checks", code)
		}
		for _, want := range []string{HealthCheckSystemdActive, HealthCheckConfigObserved, HealthCheckEndpointListening} {
			if !hasHealthCheck(contract.HealthChecks, want) {
				t.Fatalf("ContractFor(%s) missing health check %s: %#v", code, want, contract.HealthChecks)
			}
		}
	}
}

func TestOperationFromJobType(t *testing.T) {
	t.Parallel()

	op, ok := OperationFromJobType("instance.disable")
	if !ok {
		t.Fatal("instance.disable should map to an operation")
	}
	if op != OperationDisable {
		t.Fatalf("operation = %q, want %q", op, OperationDisable)
	}
	if IsInstanceOperationJobType("node.bootstrap") {
		t.Fatal("node.bootstrap must not be an instance operation job type")
	}
}

func hasHealthCheck(checks []HealthCheckSpec, code string) bool {
	for _, check := range checks {
		if check.Code == code {
			return true
		}
	}
	return false
}
