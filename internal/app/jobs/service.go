package jobs

import (
	"context"
	"errors"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

var (
	ErrTypeRequired          = errors.New("invalid job payload: type is required")
	ErrTypedEndpointRequired = errors.New("privileged job type must be created through its typed API")
	ErrGenericAPIUnavailable = errors.New("job type is not available through generic job API")
)

type PermissionError struct {
	Permission string
}

func (e PermissionError) Error() string {
	return "job type requires " + e.Permission
}

type PermissionChecker func(string) bool

type Store interface {
	CreateJob(context.Context, domain.Job) (domain.Job, error)
	ListJobs(context.Context, int) ([]domain.Job, error)
	GetJob(context.Context, string) (domain.Job, error)
	ListJobLogs(context.Context, string, int) ([]domain.JobLog, error)
	CancelJob(context.Context, string) (domain.Job, error)
}

type Service struct {
	store Store
}

func New(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, limit int) ([]domain.Job, error) {
	jobs, err := s.store.ListJobs(ctx, limit)
	if err != nil {
		return nil, err
	}
	if jobs == nil {
		jobs = []domain.Job{}
	}
	return jobs, nil
}

func (s *Service) Create(ctx context.Context, job domain.Job, can PermissionChecker) (domain.Job, error) {
	job.Type = strings.TrimSpace(job.Type)
	if job.Type == "" {
		return domain.Job{}, ErrTypeRequired
	}
	if MustUseTypedEndpoint(job.Type) {
		return domain.Job{}, ErrTypedEndpointRequired
	}
	if !AllowedInGenericAPI(job.Type) {
		return domain.Job{}, ErrGenericAPIUnavailable
	}
	if permission := RequiredPermissionForType(job.Type); permission != "" {
		if can == nil || !can(permission) {
			return domain.Job{}, PermissionError{Permission: permission}
		}
	}
	job.ScopeType = strings.TrimSpace(job.ScopeType)
	if job.ScopeType == "" {
		job.ScopeType = "system"
	}
	job.Status = "queued"
	return s.store.CreateJob(ctx, job)
}

func (s *Service) Get(ctx context.Context, id string) (domain.Job, error) {
	return s.store.GetJob(ctx, id)
}

func (s *Service) Logs(ctx context.Context, id string, limit int) ([]domain.JobLog, error) {
	return s.store.ListJobLogs(ctx, id, limit)
}

func (s *Service) Cancel(ctx context.Context, id string) (domain.Job, error) {
	return s.store.CancelJob(ctx, id)
}

func MustUseTypedEndpoint(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case "platform.control_plane_tls.apply",
		"node.bootstrap",
		"node.agent.rotate_token",
		"node.emergency_cleanup",
		"node.reboot",
		"node.backhaul.apply",
		"node.backhaul.probe",
		"node.backhaul.cleanup",
		"node.route_policy.apply",
		"node.route_policy.cleanup",
		"node.firewall.preview",
		"node.firewall.apply",
		"node.firewall.observe",
		"node.firewall.disable",
		"node.capability.install",
		"node.capability.verify",
		"instance.apply",
		"instance.restart",
		"instance.start",
		"instance.stop",
		"instance.enable",
		"instance.disable",
		"instance.diagnose",
		"instance.delete":
		return true
	default:
		return false
	}
}

func AllowedInGenericAPI(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case "artifact.build", "client.provision", "client.revoke":
		return true
	default:
		return false
	}
}

func RequiredPermissionForType(jobType string) string {
	switch strings.TrimSpace(jobType) {
	case "platform.control_plane_tls.apply":
		return "settings.manage"
	case "node.bootstrap", "node.agent.rotate_token", "node.emergency_cleanup", "node.reboot":
		return "node.bootstrap"
	case "node.capability.install", "node.capability.verify", "node.inventory", "node.inventory.sync", "node.services.discover", "node.channel.probe", "node.backhaul.apply", "node.backhaul.probe", "node.backhaul.cleanup", "node.route_policy.apply", "node.route_policy.cleanup":
		return "node.write"
	case "node.firewall.preview", "node.firewall.apply", "node.firewall.observe", "node.firewall.disable":
		return "firewall.apply"
	case "instance.apply", "instance.restart", "instance.start", "instance.stop", "instance.enable", "instance.disable", "instance.delete":
		return "instance.apply"
	case "instance.diagnose":
		return "instance.read"
	case "client.provision", "client.revoke":
		return "client.provision"
	case "artifact.build":
		return "artifact.export"
	default:
		return ""
	}
}
