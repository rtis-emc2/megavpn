package http

import "testing"

func TestRequiredPermissionForPrivilegedJobTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"instance.apply":                   "instance.apply",
		"instance.diagnose":                "instance.read",
		"instance.delete":                  "instance.apply",
		"node.backhaul.apply":              "node.write",
		"node.route_policy.apply":          "node.write",
		"node.route_policy.cleanup":        "node.write",
		"node.firewall.preview":            "firewall.apply",
		"node.firewall.apply":              "firewall.apply",
		"node.capability.install":          "node.write",
		"node.emergency_cleanup":           "node.bootstrap",
		"node.reboot":                      "node.bootstrap",
		"platform.control_plane_tls.apply": "settings.manage",
	}
	for jobType, want := range cases {
		jobType, want := jobType, want
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if got := requiredPermissionForJobType(jobType); got != want {
				t.Fatalf("permission = %q, want %q", got, want)
			}
		})
	}
}

func TestPrivilegedJobTypesMustUseTypedEndpoint(t *testing.T) {
	t.Parallel()

	for _, jobType := range []string{"instance.apply", "instance.diagnose", "instance.delete", "node.backhaul.apply", "node.route_policy.apply", "node.route_policy.cleanup", "node.firewall.preview", "node.firewall.apply", "node.capability.install", "node.emergency_cleanup", "node.reboot"} {
		jobType := jobType
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if !jobTypeMustUseTypedEndpoint(jobType) {
				t.Fatalf("expected %s to require typed endpoint", jobType)
			}
		})
	}
	if jobTypeMustUseTypedEndpoint("artifact.build") {
		t.Fatal("artifact.build should remain available to generic job API")
	}
}

func TestGenericJobAPIAllowlist(t *testing.T) {
	t.Parallel()

	for _, jobType := range []string{"artifact.build", "client.provision", "client.revoke"} {
		jobType := jobType
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if !jobTypeAllowedInGenericAPI(jobType) {
				t.Fatalf("expected %s to be allowed through generic job API", jobType)
			}
		})
	}
	for _, jobType := range []string{"node.inventory", "node.services.discover", "unknown.job"} {
		jobType := jobType
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if jobTypeAllowedInGenericAPI(jobType) {
				t.Fatalf("expected %s to be blocked by generic job API", jobType)
			}
		})
	}
}
