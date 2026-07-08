package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type memoryStore struct {
	created domain.Job
}

func (s *memoryStore) CreateJob(_ context.Context, job domain.Job) (domain.Job, error) {
	s.created = job
	job.ID = "job-1"
	return job, nil
}

func (s *memoryStore) ListJobs(context.Context, int) ([]domain.Job, error) {
	return nil, nil
}

func (s *memoryStore) GetJob(context.Context, string) (domain.Job, error) {
	return domain.Job{}, nil
}

func (s *memoryStore) ListJobLogs(context.Context, string, int) ([]domain.JobLog, error) {
	return nil, nil
}

func (s *memoryStore) CancelJob(context.Context, string) (domain.Job, error) {
	return domain.Job{}, nil
}

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
		"node.firewall.disable":            "firewall.apply",
		"node.capability.install":          "node.write",
		"node.emergency_cleanup":           "node.bootstrap",
		"node.reboot":                      "node.bootstrap",
		"platform.control_plane_tls.apply": "settings.manage",
	}
	for jobType, want := range cases {
		jobType, want := jobType, want
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if got := RequiredPermissionForType(jobType); got != want {
				t.Fatalf("permission = %q, want %q", got, want)
			}
		})
	}
}

func TestPrivilegedJobTypesMustUseTypedEndpoint(t *testing.T) {
	t.Parallel()

	for _, jobType := range []string{"instance.apply", "instance.diagnose", "instance.delete", "node.backhaul.apply", "node.route_policy.apply", "node.route_policy.cleanup", "node.firewall.preview", "node.firewall.apply", "node.firewall.disable", "node.capability.install", "node.emergency_cleanup", "node.reboot"} {
		jobType := jobType
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if !MustUseTypedEndpoint(jobType) {
				t.Fatalf("expected %s to require typed endpoint", jobType)
			}
		})
	}
	if MustUseTypedEndpoint("artifact.build") {
		t.Fatal("artifact.build should remain available to generic job API")
	}
}

func TestGenericJobAPIAllowlist(t *testing.T) {
	t.Parallel()

	for _, jobType := range []string{"artifact.build", "client.provision", "client.revoke"} {
		jobType := jobType
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if !AllowedInGenericAPI(jobType) {
				t.Fatalf("expected %s to be allowed through generic job API", jobType)
			}
		})
	}
	for _, jobType := range []string{"node.inventory", "node.services.discover", "unknown.job"} {
		jobType := jobType
		t.Run(jobType, func(t *testing.T) {
			t.Parallel()
			if AllowedInGenericAPI(jobType) {
				t.Fatalf("expected %s to be blocked by generic job API", jobType)
			}
		})
	}
}

func TestCreateDefaultsScopeAndStatus(t *testing.T) {
	t.Parallel()

	store := &memoryStore{}
	service := New(store)
	job, err := service.Create(context.Background(), domain.Job{Type: " artifact.build ", ScopeType: " "}, func(permission string) bool {
		return permission == "artifact.export"
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.Status != "queued" || job.ScopeType != "system" {
		t.Fatalf("job defaults = status %q scope %q", job.Status, job.ScopeType)
	}
	if store.created.Status != "queued" || store.created.ScopeType != "system" {
		t.Fatalf("store received defaults = status %q scope %q", store.created.Status, store.created.ScopeType)
	}
	if store.created.Type != "artifact.build" {
		t.Fatalf("store received type %q, want normalized artifact.build", store.created.Type)
	}
}

func TestCreateRejectsGenericJobWithoutPermission(t *testing.T) {
	t.Parallel()

	service := New(&memoryStore{})
	_, err := service.Create(context.Background(), domain.Job{Type: "client.provision"}, func(string) bool {
		return false
	})
	var permissionErr PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("err = %v, want PermissionError", err)
	}
	if permissionErr.Permission != "client.provision" {
		t.Fatalf("permission = %q", permissionErr.Permission)
	}
}
