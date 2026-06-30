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
		"node.capability.install":          "node.write",
		"node.emergency_cleanup":           "node.bootstrap",
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

	for _, jobType := range []string{"instance.apply", "instance.diagnose", "instance.delete", "node.backhaul.apply", "node.route_policy.apply", "node.capability.install", "node.emergency_cleanup"} {
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
