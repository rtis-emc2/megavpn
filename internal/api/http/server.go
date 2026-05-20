package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type Store interface {
	Ping(context.Context) error
	EnsureBootstrapPlatformUser(context.Context, string, string, string, string) (domain.PlatformUser, bool, error)
	GetPlatformUserForAuth(context.Context, string) (domain.PlatformUserAuth, error)
	GetPlatformUserByIDForAuth(context.Context, string) (domain.PlatformUserAuth, error)
	ListPlatformUsers(context.Context, int) ([]domain.PlatformUserRecord, error)
	CreatePlatformUser(context.Context, string, string, string, string, []string, *string) (domain.PlatformUserRecord, error)
	GetPlatformUserRecord(context.Context, string) (domain.PlatformUserRecord, error)
	GetPlatformMailSettings(context.Context) (domain.PlatformMailSettings, error)
	UpsertPlatformMailSettings(context.Context, domain.PlatformMailSettings, *string) (domain.PlatformMailSettings, error)
	MarkPlatformMailTest(context.Context, string) error
	CreatePlatformUserInvite(context.Context, string, string, string, []string, *string, time.Duration) (domain.PlatformUserRecord, domain.PlatformUserInvite, error)
	CreatePlatformUserInviteForUser(context.Context, string, string, string, string, *string, time.Duration) (domain.PlatformUserInvite, error)
	ListPlatformUserInvites(context.Context, int) ([]domain.PlatformUserInvite, error)
	GetPlatformUserInviteByToken(context.Context, string) (domain.PlatformUserInvite, error)
	MarkPlatformUserInviteDelivered(context.Context, string, string) error
	AcceptPlatformUserInvite(context.Context, string, string) (domain.PlatformUserInvite, domain.PlatformUserRecord, error)
	UpdatePlatformUserStatus(context.Context, string, string, *string) (domain.PlatformUserRecord, error)
	UpdatePlatformUserPassword(context.Context, string, string, *string) error
	DeletePlatformUser(context.Context, string, *string) error
	ListUserSessions(context.Context, int) ([]domain.UserSessionRecord, error)
	TouchPlatformUserLogin(context.Context, string) error
	CreateUserSession(context.Context, string, string, string, string, time.Time) (domain.UserSession, error)
	ResolveAuthContext(context.Context, string) (domain.AuthContext, error)
	RevokeUserSession(context.Context, string) error
	RevokeUserSessionsByUser(context.Context, string) error
	CreateAuditForUser(context.Context, *string, string, string, *string, string) (domain.AuditEvent, error)
	Dashboard(context.Context, string) (domain.Dashboard, error)
	ListNodes(context.Context) ([]domain.Node, error)
	GetNode(context.Context, string) (domain.Node, error)
	GetNodeDiagnostics(context.Context, string) (domain.NodeDiagnostics, error)
	CreateNodeAgentTokenRotateJob(context.Context, string) (domain.Job, error)
	CreateNodeChannelProbeJob(context.Context, string) (domain.Job, error)
	RequeueNodeStuckJob(context.Context, string) (domain.Job, error)
	ClearNodeStalePendingRotation(context.Context, string) ([]domain.Job, error)
	RotateNodeEnrollmentToken(context.Context, string, time.Duration) (domain.NodeEnrollmentToken, error)
	RevokeNodeAgentIdentity(context.Context, string) (domain.Node, error)
	ListNodeAccessMethods(context.Context, string) ([]domain.NodeAccessMethod, error)
	ReplaceNodeAccessMethods(context.Context, string, []domain.NodeAccessMethod) ([]domain.NodeAccessMethod, error)
	CreateNodeBootstrapJob(context.Context, string, string, map[string]any) (domain.Job, domain.NodeBootstrapRun, error)
	ListNodeBootstrapRuns(context.Context, string, int) ([]domain.NodeBootstrapRun, error)
	LatestNodeInventory(context.Context, string) (domain.NodeInventorySnapshot, error)
	ListNodeCapabilities(context.Context, string) ([]domain.NodeCapability, error)
	ListNodeCapabilityInstallEvents(context.Context, string, int) ([]domain.NodeCapabilityInstallEvent, error)
	CreateNodeCapabilityInstallJob(context.Context, string, string, string, string) (domain.Job, error)
	CreateNodeCapabilityVerifyJob(context.Context, string, string) (domain.Job, error)
	SubmitNodeInventory(context.Context, string, map[string]any) (domain.NodeInventorySnapshot, []domain.NodeCapability, error)
	ListNodeServiceDiscoveries(context.Context, string) ([]domain.NodeServiceDiscovery, error)
	GetNodeServiceDiscovery(context.Context, string, string) (domain.NodeServiceDiscovery, error)
	NodeServiceDiscoverySummary(context.Context, string) (domain.NodeServiceDiscoverySummary, error)
	CreateNodeServiceDiscoveryJob(context.Context, string) (domain.Job, error)
	IgnoreNodeServiceDiscovery(context.Context, string, string, bool) (domain.NodeServiceDiscovery, error)
	ImportNodeServiceDiscovery(context.Context, string, string) (domain.Instance, error)
	ImportAllNodeServiceDiscoveries(context.Context, string) ([]domain.Instance, error)
	CreateNodeInventoryJob(context.Context, string) (domain.Job, error)
	CreateNode(context.Context, domain.Node) (domain.Node, error)
	RetireNode(context.Context, string) (domain.Node, error)
	SetNodeMaintenance(context.Context, string, bool) (domain.Node, error)
	ListServiceDefinitions(context.Context) ([]domain.ServiceDefinition, error)
	ListInstances(context.Context) ([]domain.Instance, error)
	GetInstance(context.Context, string) (domain.Instance, error)
	GetInstanceWithSpec(context.Context, string) (domain.Instance, error)
	ListInstanceRevisions(context.Context, string, int) ([]domain.InstanceRevision, error)
	CreateInstance(context.Context, domain.Instance) (domain.Instance, error)
	ReplaceInstanceSpec(context.Context, string, string, map[string]any) (domain.InstanceRevision, error)
	UpdateInstanceStatus(context.Context, string, string) (domain.Job, error)
	DeleteInstance(context.Context, string) (domain.Instance, error)
	ListClients(context.Context) ([]domain.Client, error)
	GetClient(context.Context, string) (domain.Client, error)
	CreateClient(context.Context, domain.Client) (domain.Client, error)
	SetClientStatus(context.Context, string, string) (domain.Client, error)
	ProvisionClient(context.Context, string, []string) (domain.Job, error)
	RevokeClient(context.Context, string) (domain.Job, error)
	ListServiceAccesses(context.Context, string) ([]domain.ServiceAccess, error)
	RotateServiceAccess(context.Context, string, string, string) (domain.Job, error)
	ListArtifacts(context.Context, string) ([]domain.Artifact, error)
	PublishShareLink(context.Context, string, string, time.Duration) (domain.ShareLink, error)
	ListShareLinks(context.Context, string) ([]domain.ShareLink, error)
	RevokeShareLink(context.Context, string, string) (domain.ShareLink, error)
	ResolveShareLinkArtifact(context.Context, string) (domain.ShareLink, domain.Artifact, error)
	CreateSecretRef(context.Context, string, []byte, map[string]any) (domain.SecretRef, error)
	ResolveSecretValue(context.Context, string) (domain.SecretRef, []byte, error)
	CreateClientEmailDelivery(context.Context, domain.ClientEmailDelivery) (domain.ClientEmailDelivery, error)
	UpdateClientEmailDeliveryStatus(context.Context, string, string, string, map[string]any) error
	CreateJob(context.Context, domain.Job) (domain.Job, error)
	ListJobs(context.Context, int) ([]domain.Job, error)
	GetJob(context.Context, string) (domain.Job, error)
	ListJobLogs(context.Context, string, int) ([]domain.JobLog, error)
	CancelJob(context.Context, string) (domain.Job, error)
	ListAudit(context.Context, int) ([]domain.AuditEvent, error)
	ListPlatformServicePKIRoots(context.Context) ([]domain.PlatformServicePKIRoot, error)
	CreateNodeEnrollmentToken(context.Context, string, time.Duration) (domain.NodeEnrollmentToken, error)
	ListNodeEnrollmentTokens(context.Context, string) ([]domain.NodeEnrollmentToken, error)
	RegisterAgentWithEnrollment(context.Context, string, string, string, string) (domain.Node, string, error)
	UpsertAgentNode(context.Context, string, string, string) (domain.Node, error)
	ValidateAgentToken(context.Context, string, string) bool
	ValidateAgentTokenForJob(context.Context, string, string) bool
	RecordAgentAuthFailure(context.Context, string, string) error
	TouchAgentJobPoll(context.Context, string) error
	RecordAgentJobClaim(context.Context, string, string, string) error
	RecordAgentJobResult(context.Context, string, string, string, string) error
	RecordAgentInventorySync(context.Context, string, string) error
	HeartbeatByNodeID(context.Context, string) error
	Heartbeat(context.Context, string) error
	AgentNextJob(context.Context, string) (domain.Job, bool, error)
	CompleteJob(context.Context, string, string, map[string]any) error
	AddJobLog(context.Context, string, string, string, map[string]any) error
}

type Server struct {
	log                 *slog.Logger
	store               Store
	version             string
	publicBaseURL       string
	agentToken          string
	allowAutoRegister   bool
	sessionTTL          time.Duration
	sessionCookieName   string
	sessionCookieSecure bool
	webRoot             string
	rateLimiter         *rateLimiter
	trustProxyHeaders   bool
	maxRequestBytes     int64
}

type Options struct {
	Version             string
	PublicBaseURL       string
	AgentToken          string
	AllowAutoRegister   bool
	SessionTTL          time.Duration
	SessionCookieName   string
	SessionCookieSecure bool
	WebRoot             string
	TrustProxyHeaders   bool
	MaxRequestBytes     int64
}

func New(log *slog.Logger, store Store, opts Options) nethttp.Handler {
	s := &Server{
		log:                 log,
		store:               store,
		version:             opts.Version,
		publicBaseURL:       strings.TrimRight(strings.TrimSpace(opts.PublicBaseURL), "/"),
		agentToken:          opts.AgentToken,
		allowAutoRegister:   opts.AllowAutoRegister,
		sessionTTL:          opts.SessionTTL,
		sessionCookieName:   opts.SessionCookieName,
		sessionCookieSecure: opts.SessionCookieSecure,
		webRoot:             strings.TrimSpace(opts.WebRoot),
		rateLimiter:         newRateLimiter(),
		trustProxyHeaders:   opts.TrustProxyHeaders,
		maxRequestBytes:     opts.MaxRequestBytes,
	}
	mux := nethttp.NewServeMux()
	protected := func(pattern, permission string, h func(nethttp.ResponseWriter, *nethttp.Request)) {
		mux.Handle(pattern, s.withPermission(permission, nethttp.HandlerFunc(h)))
	}
	authenticated := func(pattern string, h func(nethttp.ResponseWriter, *nethttp.Request)) {
		mux.Handle(pattern, s.withPermission("", nethttp.HandlerFunc(h)))
	}
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /share/{token}", s.withRateLimit("public_share_download", 120, time.Minute, s.publicShareDownload))
	mux.HandleFunc("GET /assets/{path...}", s.assets)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/ready", s.ready)
	mux.HandleFunc("GET /api/v1/version", s.versionHandler)
	mux.HandleFunc("GET /api/v1/auth/invites/{token}", s.getPlatformInvitePublic)
	mux.HandleFunc("POST /api/v1/auth/invites/{token}/accept", s.withRateLimit("invite_accept", 10, time.Minute, s.acceptPlatformInvite))
	mux.HandleFunc("POST /api/v1/auth/login", s.withRateLimit("auth_login", 10, time.Minute, s.login))
	authenticated("POST /api/v1/auth/logout", s.logout)
	authenticated("GET /api/v1/auth/me", s.me)
	authenticated("POST /api/v1/auth/change-password", s.changePassword)
	protected("GET /api/v1/admin/users", "auth.manage", s.listPlatformUsers)
	protected("POST /api/v1/admin/users", "auth.manage", s.createPlatformUser)
	protected("POST /api/v1/admin/users/invite", "auth.manage", s.invitePlatformUser)
	protected("GET /api/v1/admin/user-invites", "auth.manage", s.listPlatformUserInvites)
	protected("POST /api/v1/admin/users/{id}/status", "auth.manage", s.updatePlatformUserStatus)
	protected("POST /api/v1/admin/users/{id}/reset-password", "auth.manage", s.resetPlatformUserPassword)
	protected("POST /api/v1/admin/users/{id}/resend-invite", "auth.manage", s.resendPlatformUserInvite)
	protected("DELETE /api/v1/admin/users/{id}", "auth.manage", s.deletePlatformUser)
	protected("GET /api/v1/admin/sessions", "auth.manage", s.listUserSessions)
	protected("POST /api/v1/admin/sessions/{id}/revoke", "auth.manage", s.revokePlatformSession)
	protected("GET /api/v1/settings/mail", "auth.manage", s.getPlatformMailSettings)
	protected("PUT /api/v1/settings/mail", "auth.manage", s.updatePlatformMailSettings)
	protected("POST /api/v1/settings/mail/test", "auth.manage", s.sendPlatformMailTest)
	protected("GET /api/v1/platform/pki-roots", "instance.read", s.listPlatformServicePKIRoots)
	protected("POST /api/v1/secret-refs", "node.bootstrap", s.createSecretRef)
	protected("GET /api/v1/dashboard", "dashboard.read", s.dashboard)
	protected("GET /api/v1/services", "service.read", s.listServices)
	protected("GET /api/v1/services/installers", "service.read", s.listServiceInstallers)
	protected("GET /api/v1/nodes", "node.read", s.listNodes)
	protected("POST /api/v1/nodes", "node.write", s.createNode)
	protected("GET /api/v1/nodes/{id}", "node.read", s.getNode)
	protected("GET /api/v1/nodes/{id}/diagnostics", "node.read", s.getNodeDiagnostics)
	protected("POST /api/v1/nodes/{id}/diagnostics/retry-inventory", "node.write", s.retryNodeInventorySync)
	protected("POST /api/v1/nodes/{id}/diagnostics/retry-discovery", "node.write", s.retryNodeDiscoverySync)
	protected("POST /api/v1/nodes/{id}/diagnostics/requeue-stuck-job", "node.write", s.requeueNodeStuckJob)
	protected("POST /api/v1/nodes/{id}/diagnostics/channel-probe", "node.write", s.probeNodeChannel)
	protected("POST /api/v1/nodes/{id}/diagnostics/clear-stale-rotation", "node.bootstrap", s.clearNodeStaleRotation)
	protected("GET /api/v1/nodes/{id}/access-methods", "node.read", s.listNodeAccessMethods)
	protected("PUT /api/v1/nodes/{id}/access-methods", "node.bootstrap", s.replaceNodeAccessMethods)
	protected("POST /api/v1/nodes/{id}/bootstrap", "node.bootstrap", s.createNodeBootstrapJob)
	protected("GET /api/v1/nodes/{id}/bootstrap-runs", "node.read", s.listNodeBootstrapRuns)
	protected("POST /api/v1/nodes/{id}/agent-token/rotate", "node.bootstrap", s.rotateNodeAgentToken)
	protected("POST /api/v1/nodes/{id}/enrollment-token/rotate", "node.bootstrap", s.rotateNodeEnrollmentToken)
	protected("POST /api/v1/nodes/{id}/agent-identity/revoke", "node.bootstrap", s.revokeNodeAgentIdentity)
	protected("GET /api/v1/nodes/{id}/inventory", "node.read", s.getNodeInventory)
	protected("GET /api/v1/nodes/{id}/capabilities", "node.read", s.getNodeCapabilities)
	protected("POST /api/v1/nodes/{id}/capabilities/install", "node.write", s.installNodeCapability)
	protected("POST /api/v1/nodes/{id}/capabilities/verify", "node.write", s.verifyNodeCapability)
	protected("GET /api/v1/nodes/{id}/capabilities/drift", "node.read", s.nodeCapabilitiesDrift)
	protected("GET /api/v1/nodes/{id}/capabilities/install-events", "node.read", s.nodeCapabilityInstallEvents)
	protected("GET /api/v1/nodes/{id}/services/discovered", "node.read", s.getNodeServiceDiscoveries)
	protected("GET /api/v1/nodes/{id}/services/discovery-summary", "node.read", s.getNodeServiceDiscoverySummary)
	protected("GET /api/v1/nodes/{id}/services/discovered/{discovery_id}", "node.read", s.getNodeServiceDiscovery)
	protected("POST /api/v1/nodes/{id}/services/discovered/{discovery_id}/ignore", "node.write", s.ignoreNodeServiceDiscovery)
	protected("POST /api/v1/nodes/{id}/services/discovered/{discovery_id}/unignore", "node.write", s.unignoreNodeServiceDiscovery)
	protected("POST /api/v1/nodes/{id}/services/discovered/{discovery_id}/import", "node.write", s.importNodeServiceDiscovery)
	protected("POST /api/v1/nodes/{id}/services/import-all", "node.write", s.importAllNodeServiceDiscoveries)
	protected("POST /api/v1/nodes/{id}/services/discover", "node.write", s.createNodeServiceDiscoveryJob)
	protected("POST /api/v1/nodes/{id}/inventory/sync", "node.write", s.createNodeInventoryJob)
	protected("DELETE /api/v1/nodes/{id}", "node.write", s.retireNode)
	protected("POST /api/v1/nodes/{id}/enrollment-token", "node.bootstrap", s.createNodeEnrollmentToken)
	protected("GET /api/v1/nodes/{id}/enrollment-tokens", "node.read", s.listNodeEnrollmentTokens)
	protected("POST /api/v1/nodes/{id}/maintenance/enable", "node.write", s.nodeMaintenanceEnable)
	protected("POST /api/v1/nodes/{id}/maintenance/disable", "node.write", s.nodeMaintenanceDisable)
	protected("GET /api/v1/instances", "instance.read", s.listInstances)
	protected("POST /api/v1/instances", "instance.write", s.createInstance)
	protected("GET /api/v1/instances/{id}", "instance.read", s.getInstance)
	protected("GET /api/v1/instances/{id}/revisions", "instance.read", s.listInstanceRevisions)
	protected("PUT /api/v1/instances/{id}/spec", "instance.write", s.replaceInstanceSpec)
	protected("DELETE /api/v1/instances/{id}", "instance.write", s.deleteInstance)
	protected("POST /api/v1/instances/{id}/apply", "instance.apply", s.instanceAction("apply"))
	protected("POST /api/v1/instances/{id}/restart", "instance.apply", s.instanceAction("restart"))
	protected("POST /api/v1/instances/{id}/start", "instance.apply", s.instanceAction("start"))
	protected("POST /api/v1/instances/{id}/stop", "instance.apply", s.instanceAction("stop"))
	protected("POST /api/v1/instances/{id}/enable", "instance.apply", s.instanceAction("enable"))
	protected("POST /api/v1/instances/{id}/disable", "instance.apply", s.instanceAction("disable"))
	protected("GET /api/v1/clients", "client.read", s.listClients)
	protected("POST /api/v1/clients", "client.write", s.createClient)
	protected("GET /api/v1/clients/{id}", "client.read", s.getClient)
	protected("DELETE /api/v1/clients/{id}", "client.write", s.deleteClient)
	protected("POST /api/v1/clients/{id}/provision", "client.provision", s.provisionClient)
	protected("POST /api/v1/clients/{id}/revoke", "client.provision", s.revokeClient)
	protected("POST /api/v1/clients/{id}/suspend", "client.write", s.clientStatus("suspended"))
	protected("POST /api/v1/clients/{id}/activate", "client.write", s.clientStatus("active"))
	protected("GET /api/v1/clients/{id}/accesses", "client.read", s.clientAccesses)
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-openvpn", "client.provision", s.rotateClientAccess("openvpn"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-xray", "client.provision", s.rotateClientAccess("xray-core"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-wireguard", "client.provision", s.rotateClientAccess("wireguard"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-mtproto", "client.provision", s.rotateClientAccess("mtproto"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-ipsec", "client.provision", s.rotateClientAccess("ipsec"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-http-proxy", "client.provision", s.rotateClientAccess("http_proxy"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-shadowsocks", "client.provision", s.rotateClientAccess("shadowsocks"))
	protected("GET /api/v1/clients/{id}/artifacts", "artifact.read", s.clientArtifacts)
	protected("POST /api/v1/clients/{id}/artifacts", "artifact.export", s.createArtifact)
	protected("GET /api/v1/clients/{id}/share-links", "artifact.read", s.clientShareLinks)
	protected("POST /api/v1/clients/{id}/share-links", "share_link.manage", s.publishShareLink)
	protected("POST /api/v1/clients/{id}/share-links/{link_id}/revoke", "share_link.manage", s.revokeShareLink)
	protected("POST /api/v1/clients/{id}/deliver-email", "artifact.export", s.deliverClientEmail)
	protected("GET /api/v1/artifacts", "artifact.read", s.listArtifacts)
	protected("GET /api/v1/share-links", "artifact.read", s.listShareLinks)
	protected("GET /api/v1/jobs", "job.read", s.listJobs)
	protected("POST /api/v1/jobs", "job.write", s.createJob)
	protected("GET /api/v1/jobs/{id}", "job.read", s.getJob)
	protected("GET /api/v1/jobs/{id}/logs", "job.read", s.jobLogs)
	protected("POST /api/v1/jobs/{id}/cancel", "job.cancel", s.cancelJob)
	protected("GET /api/v1/audit", "audit.read", s.listAudit)
	mux.HandleFunc("POST /agent/register", s.withRateLimit("agent_register", 30, time.Minute, s.agentRegister))
	mux.HandleFunc("POST /agent/heartbeat", s.agentHeartbeat)
	mux.HandleFunc("POST /agent/inventory", s.agentInventory)
	mux.HandleFunc("GET /agent/jobs/next", s.agentNextJob)
	mux.HandleFunc("POST /agent/jobs/{id}/result", s.agentJobResult)
	handler := nethttp.Handler(mux)
	if s.maxRequestBytes > 0 {
		handler = nethttp.MaxBytesHandler(handler, s.maxRequestBytes)
	}
	return recoverer(log, requestLogger(log, securityHeaders(strings.HasPrefix(s.publicBaseURL, "https://"), handler)))
}

type response map[string]any

type provisionRequest struct {
	InstanceIDs []string `json:"instance_ids"`
}
type artifactRequest struct {
	Type        string   `json:"type"`
	InstanceIDs []string `json:"instance_ids"`
}
type instanceSpecRequest struct {
	Spec map[string]any `json:"spec"`
}
type shareLinkRequest struct {
	TargetID string `json:"target_id"`
	TTLHours int    `json:"ttl_hours"`
}

type capabilityInstallRequest struct {
	ServiceCode string `json:"service_code"`
	Strategy    string `json:"strategy"`
	Channel     string `json:"channel"`
}

type capabilityVerifyRequest struct {
	ServiceCode string `json:"service_code"`
}

type nodeAccessMethodsRequest struct {
	Items []domain.NodeAccessMethod `json:"items"`
}

type nodeBootstrapRequest struct {
	BootstrapMode  string `json:"bootstrap_mode"`
	ReinstallAgent bool   `json:"reinstall_agent"`
	ForceReenroll  bool   `json:"force_reenroll"`
}

type secretRefCreateRequest struct {
	SecretType string         `json:"secret_type"`
	Value      string         `json:"value"`
	Meta       map[string]any `json:"meta"`
}

func writeJSON(w nethttp.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErr(w nethttp.ResponseWriter, code int, msg string) {
	writeJSON(w, code, response{"status": "error", "error": msg})
}
func decode(r *nethttp.Request, v any) bool {
	return decodeJSONBody(r, v, false)
}

func decodeOptional(r *nethttp.Request, v any) bool {
	return decodeJSONBody(r, v, true)
}

func decodeJSONBody(r *nethttp.Request, v any, optional bool) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if optional && err == io.EOF {
			return true
		}
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return false
	}
	return true
}

func idParam(r *nethttp.Request) string { return strings.TrimSpace(r.PathValue("id")) }

func (s *Server) index(w nethttp.ResponseWriter, r *nethttp.Request) {
	if s.serveFileIfExists(w, r, "index.html", "text/html; charset=utf-8") {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>RTIS MegaVPN API</title><style>body{font-family:system-ui;background:#0b1120;color:#e5e7eb;padding:40px}a{color:#fda4af}code{background:#111827;padding:2px 6px;border-radius:6px}</style></head><body><h1>RTIS MegaVPN Control Plane</h1><p>API is running.</p><p><a href="/health">/health</a> · <a href="/api/v1/dashboard">/api/v1/dashboard</a> · <a href="/api/v1/ready">/api/v1/ready</a></p></body></html>`))
}

func (s *Server) assets(w nethttp.ResponseWriter, r *nethttp.Request) {
	assetPath := strings.TrimSpace(r.PathValue("path"))
	if assetPath == "" || strings.HasSuffix(assetPath, "/") {
		nethttp.NotFound(w, r)
		return
	}
	if s.serveFileIfExists(w, r, filepath.Join("assets", assetPath), "") {
		return
	}
	nethttp.NotFound(w, r)
}

func (s *Server) serveFileIfExists(w nethttp.ResponseWriter, r *nethttp.Request, relPath, contentType string) bool {
	relPath = strings.TrimLeft(strings.TrimSpace(relPath), "/")
	if relPath == "" || strings.Contains(relPath, "..") {
		return false
	}
	if absPath, ok := s.resolveWebAsset(relPath); ok {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		nethttp.ServeFile(w, r, absPath)
		return true
	}
	return false
}

func (s *Server) resolveWebAsset(relPath string) (string, bool) {
	for _, root := range candidateWebRoots(s.webRoot) {
		absPath := filepath.Clean(filepath.Join(root, relPath))
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			continue
		}
		return absPath, true
	}
	return "", false
}

func candidateWebRoots(configured string) []string {
	candidates := []string{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}
	add(configured)
	add("web")
	add("/opt/megavpn/web")
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		add(filepath.Join(exeDir, "web"))
		add(filepath.Join(exeDir, "..", "web"))
	}
	return candidates
}
func (s *Server) health(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, response{"status": "ok", "service": "megavpn-api", "version": s.version, "time": time.Now().UTC().Format(time.RFC3339)})
}
func (s *Server) ready(w nethttp.ResponseWriter, r *nethttp.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeErr(w, 503, "database is not ready")
		return
	}
	writeJSON(w, 200, response{"status": "ready", "service": "megavpn-api", "time": time.Now().UTC().Format(time.RFC3339)})
}
func (s *Server) versionHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, response{"service": "megavpn-api", "version": s.version, "time": time.Now().UTC().Format(time.RFC3339)})
}
func (s *Server) dashboard(w nethttp.ResponseWriter, r *nethttp.Request) {
	d, err := s.store.Dashboard(r.Context(), s.version)
	if err != nil {
		writeErr(w, 500, "dashboard failed")
		return
	}
	writeJSON(w, 200, d)
}
func (s *Server) listServices(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListServiceDefinitions(r.Context())
	if err != nil {
		writeErr(w, 500, "list services failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listServiceInstallers(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, []response{
		{"service_code": "nginx", "strategy": "nginx_org_repo", "channel": "stable", "description": "Install nginx from the official nginx.org Ubuntu repository; falls back to ubuntu_repo when CPU ISA is incompatible."},
		{"service_code": "nginx", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install nginx from the Ubuntu repository."},
		{"service_code": "nginx", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed nginx."},
		{"service_code": "xray-core", "strategy": "xtls_install_release", "channel": "latest", "description": "Install Xray-core through the official XTLS/Xray-install release script."},
		{"service_code": "xray-core", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed xray-core."},
		{"service_code": "openvpn", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install OpenVPN from the Ubuntu repository and register the capability on the node."},
		{"service_code": "openvpn", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed OpenVPN runtime."},
		{"service_code": "wireguard", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install WireGuard userspace tooling from the Ubuntu repository."},
		{"service_code": "wireguard", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed WireGuard runtime."},
		{"service_code": "ipsec", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install strongSwan / IPsec packages from the Ubuntu repository."},
		{"service_code": "ipsec", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed strongSwan / IPsec runtime."},
		{"service_code": "http_proxy", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install Squid HTTP proxy from the Ubuntu repository."},
		{"service_code": "http_proxy", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed Squid runtime."},
		{"service_code": "xl2tpd", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install xl2tpd from the Ubuntu repository for L2TP support."},
		{"service_code": "xl2tpd", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed xl2tpd runtime."},
		{"service_code": "shadowsocks", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install shadowsocks-libev from the Ubuntu repository."},
		{"service_code": "shadowsocks", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed Shadowsocks runtime."},
	})
}

func (s *Server) listNodes(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListNodes(r.Context())
	if err != nil {
		writeErr(w, 500, "list nodes failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) getNode(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetNode(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "node not found")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) listNodeAccessMethods(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListNodeAccessMethods(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list node access methods failed")
		return
	}
	if x == nil {
		x = []domain.NodeAccessMethod{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) replaceNodeAccessMethods(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeAccessMethodsRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid access methods payload")
		return
	}
	x, err := s.store.ReplaceNodeAccessMethods(r.Context(), idParam(r), req.Items)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) createNodeBootstrapJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeBootstrapRequest
	if r.ContentLength > 0 && !decode(r, &req) {
		writeErr(w, 400, "invalid bootstrap payload")
		return
	}
	options := map[string]any{
		"reinstall_agent": req.ReinstallAgent,
		"force_reenroll":  req.ForceReenroll,
	}
	job, run, err := s.store.CreateNodeBootstrapJob(r.Context(), idParam(r), req.BootstrapMode, options)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{"job": job, "bootstrap_run": run})
}

func (s *Server) listNodeBootstrapRuns(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListNodeBootstrapRuns(r.Context(), idParam(r), limit)
	if err != nil {
		writeErr(w, 500, "list node bootstrap runs failed")
		return
	}
	if x == nil {
		x = []domain.NodeBootstrapRun{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) getNodeInventory(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.LatestNodeInventory(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "node inventory not found")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) getNodeCapabilities(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListNodeCapabilities(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list node capabilities failed")
		return
	}
	if x == nil {
		x = []domain.NodeCapability{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) installNodeCapability(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req capabilityInstallRequest
	if !decode(r, &req) || strings.TrimSpace(req.ServiceCode) == "" {
		writeErr(w, 400, "invalid capability install payload: service_code is required")
		return
	}
	j, err := s.store.CreateNodeCapabilityInstallJob(r.Context(), idParam(r), req.ServiceCode, req.Strategy, req.Channel)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 202, j)
}

func (s *Server) verifyNodeCapability(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req capabilityVerifyRequest
	if !decode(r, &req) || strings.TrimSpace(req.ServiceCode) == "" {
		writeErr(w, 400, "invalid capability verify payload: service_code is required")
		return
	}
	j, err := s.store.CreateNodeCapabilityVerifyJob(r.Context(), idParam(r), req.ServiceCode)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 202, j)
}

func (s *Server) nodeCapabilityInstallEvents(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListNodeCapabilityInstallEvents(r.Context(), idParam(r), limit)
	if err != nil {
		writeErr(w, 500, "list capability install events failed")
		return
	}
	if x == nil {
		x = []domain.NodeCapabilityInstallEvent{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) nodeCapabilitiesDrift(w nethttp.ResponseWriter, r *nethttp.Request) {
	caps, err := s.store.ListNodeCapabilities(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list node capabilities failed")
		return
	}
	required := []string{"nginx", "xray-core"}
	byCode := map[string]domain.NodeCapability{}
	for _, c := range caps {
		byCode[c.CapabilityCode] = c
	}
	drift := []response{}
	for _, code := range required {
		c, ok := byCode[code]
		actual := "missing"
		if ok {
			actual = c.Status
		}
		drift = append(drift, response{"capability_code": code, "desired": "available", "actual": actual, "in_sync": actual == "available"})
	}
	writeJSON(w, 200, response{"node_id": idParam(r), "required": required, "drift": drift})
}

func (s *Server) getNodeServiceDiscoveries(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListNodeServiceDiscoveries(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list node service discoveries failed")
		return
	}
	if x == nil {
		x = []domain.NodeServiceDiscovery{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) getNodeServiceDiscoverySummary(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.NodeServiceDiscoverySummary(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "node service discovery summary not found")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) getNodeServiceDiscovery(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetNodeServiceDiscovery(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("discovery_id")))
	if err != nil {
		writeErr(w, 404, "node service discovery not found")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) ignoreNodeServiceDiscovery(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.IgnoreNodeServiceDiscovery(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("discovery_id")), true)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) unignoreNodeServiceDiscovery(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.IgnoreNodeServiceDiscovery(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("discovery_id")), false)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) importNodeServiceDiscovery(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ImportNodeServiceDiscovery(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("discovery_id")))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, x)
}

func (s *Server) importAllNodeServiceDiscoveries(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ImportAllNodeServiceDiscoveries(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if x == nil {
		x = []domain.Instance{}
	}
	writeJSON(w, 201, x)
}

func (s *Server) createNodeServiceDiscoveryJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.CreateNodeServiceDiscoveryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, x)
}

func (s *Server) createNodeInventoryJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	j, err := s.store.CreateNodeInventoryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, j)
}

func (s *Server) createNode(w nethttp.ResponseWriter, r *nethttp.Request) {
	var n domain.Node
	if !decode(r, &n) || n.Name == "" || n.Address == "" {
		writeErr(w, 400, "invalid node payload: name and address are required")
		return
	}
	if n.Role != "" && n.Role != "ingress" && n.Role != "egress" {
		writeErr(w, 400, "invalid node payload: role must be ingress or egress")
		return
	}
	x, err := s.store.CreateNode(r.Context(), n)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, x)
}
func (s *Server) retireNode(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.RetireNode(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) createNodeEnrollmentToken(w nethttp.ResponseWriter, r *nethttp.Request) {
	ttl := 24 * time.Hour
	if raw := r.URL.Query().Get("ttl_hours"); raw != "" {
		if h, err := strconv.Atoi(raw); err == nil && h > 0 && h <= 720 {
			ttl = time.Duration(h) * time.Hour
		}
	}
	x, err := s.store.CreateNodeEnrollmentToken(r.Context(), idParam(r), ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, x)
}
func (s *Server) listNodeEnrollmentTokens(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListNodeEnrollmentTokens(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list enrollment tokens failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) nodeMaintenanceEnable(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.SetNodeMaintenance(r.Context(), idParam(r), true)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) nodeMaintenanceDisable(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.SetNodeMaintenance(r.Context(), idParam(r), false)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listInstances(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListInstances(r.Context())
	if err != nil {
		writeErr(w, 500, "list instances failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) getInstance(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetInstanceWithSpec(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "instance not found")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listInstanceRevisions(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListInstanceRevisions(r.Context(), idParam(r), limit)
	if err != nil {
		writeErr(w, 500, "list instance revisions failed")
		return
	}
	if x == nil {
		x = []domain.InstanceRevision{}
	}
	writeJSON(w, 200, x)
}
func (s *Server) replaceInstanceSpec(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req instanceSpecRequest
	if !decode(r, &req) || req.Spec == nil {
		writeErr(w, 400, "invalid instance spec payload")
		return
	}
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	source := strings.TrimSpace(authCtx.User.Username)
	if source == "" {
		source = strings.TrimSpace(authCtx.User.Email)
	}
	revision, err := s.store.ReplaceInstanceSpec(r.Context(), idParam(r), source, req.Spec)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	instanceID := idParam(r)
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "instance.revision.replace", "instance", &instanceID, "instance spec revision replaced")
	writeJSON(w, 200, response{"revision": revision})
}
func (s *Server) createInstance(w nethttp.ResponseWriter, r *nethttp.Request) {
	var x domain.Instance
	if !decode(r, &x) || x.NodeID == "" || x.ServiceCode == "" || x.Name == "" {
		writeErr(w, 400, "invalid instance payload: node_id, service_code and name are required")
		return
	}
	created, err := s.store.CreateInstance(r.Context(), x)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, created)
}
func (s *Server) deleteInstance(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.DeleteInstance(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) instanceAction(action string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		j, err := s.store.UpdateInstanceStatus(r.Context(), idParam(r), action)
		if err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		writeJSON(w, 202, j)
	}
}
func (s *Server) listClients(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListClients(r.Context())
	if err != nil {
		writeErr(w, 500, "list clients failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) getClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetClient(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "client not found")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) createClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	var c domain.Client
	if !decode(r, &c) || c.Username == "" {
		writeErr(w, 400, "invalid client payload: username is required")
		return
	}
	x, err := s.store.CreateClient(r.Context(), c)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, x)
}
func (s *Server) deleteClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.SetClientStatus(r.Context(), idParam(r), "deleted")
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) clientStatus(status string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		x, err := s.store.SetClientStatus(r.Context(), idParam(r), status)
		if err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, x)
	}
}
func (s *Server) provisionClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req provisionRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid provision payload")
		return
	}
	j, err := s.store.ProvisionClient(r.Context(), idParam(r), req.InstanceIDs)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, j)
}
func (s *Server) revokeClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	j, err := s.store.RevokeClient(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, j)
}
func (s *Server) clientAccesses(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListServiceAccesses(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list accesses failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) rotateClientAccess(driver string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		jobRecord, err := s.store.RotateServiceAccess(r.Context(), idParam(r), r.PathValue("access_id"), driver)
		if err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		writeJSON(w, 202, jobRecord)
	}
}
func (s *Server) clientArtifacts(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListArtifacts(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list artifacts failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) createArtifact(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req artifactRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid artifact payload")
		return
	}
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	switch req.Type {
	case "", "all", "zip_bundle", "ovpn", "vless_url", "wg_conf", "mtproto_url", "http_proxy_bundle", "ss_url", "ipsec_bundle":
	default:
		writeErr(w, 400, "unsupported artifact type")
		return
	}
	jobRecord, err := s.store.ProvisionClient(r.Context(), idParam(r), req.InstanceIDs)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"job":            jobRecord,
		"requested_type": req.Type,
		"message":        "artifact export queues client.provision; the worker will build the supported artifacts for the selected instances, including ovpn, vless_url, wg_conf, mtproto_url, http_proxy_bundle, ss_url and ipsec_bundle for the current service drivers",
	})
}
func (s *Server) clientShareLinks(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListShareLinks(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list share links failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) publishShareLink(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req shareLinkRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid share link payload")
		return
	}
	ttl := time.Duration(req.TTLHours) * time.Hour
	x, err := s.store.PublishShareLink(r.Context(), idParam(r), req.TargetID, ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, x)
}
func (s *Server) revokeShareLink(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.RevokeShareLink(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("link_id")))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listArtifacts(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListArtifacts(r.Context(), "")
	if err != nil {
		writeErr(w, 500, "list artifacts failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listShareLinks(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListShareLinks(r.Context(), "")
	if err != nil {
		writeErr(w, 500, "list share links failed")
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listJobs(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListJobs(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, "list jobs failed")
		return
	}
	if x == nil {
		x = []domain.Job{}
	}
	writeJSON(w, 200, x)
}
func (s *Server) createJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	var j domain.Job
	if !decode(r, &j) || j.Type == "" {
		writeErr(w, 400, "invalid job payload: type is required")
		return
	}
	if j.ScopeType == "" {
		j.ScopeType = "system"
	}
	x, err := s.store.CreateJob(r.Context(), j)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 202, x)
}

func (s *Server) getJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "job not found")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) jobLogs(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListJobLogs(r.Context(), idParam(r), limit)
	if err != nil {
		writeErr(w, 500, "list job logs failed")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) cancelJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.CancelJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) listAudit(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListAudit(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, "list audit failed")
		return
	}
	writeJSON(w, 200, x)
}

type agentRegisterReq struct {
	NodeID          string `json:"node_id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Token           string `json:"token"`
	EnrollmentToken string `json:"enrollment_token"`
}
type agentHeartbeatReq struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
}
type agentInventoryReq struct {
	NodeID    string         `json:"node_id"`
	Source    string         `json:"source"`
	Inventory map[string]any `json:"inventory"`
}
type agentResultReq struct {
	Status string         `json:"status"`
	Result map[string]any `json:"result"`
}

func bearerToken(r *nethttp.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func (s *Server) authorizeAgentBootstrap(r *nethttp.Request, token string) bool {
	authToken := bearerToken(r)
	return s.agentToken != "" && (authToken == s.agentToken || (token != "" && token == s.agentToken))
}

func (s *Server) authorizeAgentNode(r *nethttp.Request, nodeID string) bool {
	tok := bearerToken(r)
	if tok == "" || nodeID == "" {
		return false
	}
	if s.store != nil && s.store.ValidateAgentToken(r.Context(), nodeID, tok) {
		return true
	}
	return s.allowAutoRegister && s.agentToken != "" && tok == s.agentToken
}

func (s *Server) authorizeAgentJob(r *nethttp.Request, jobID string) bool {
	tok := bearerToken(r)
	if tok == "" || jobID == "" {
		return false
	}
	if s.store != nil && s.store.ValidateAgentTokenForJob(r.Context(), jobID, tok) {
		return true
	}
	return s.allowAutoRegister && s.agentToken != "" && tok == s.agentToken
}
func (s *Server) agentRegister(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req agentRegisterReq
	if !decode(r, &req) {
		writeErr(w, 400, "invalid agent register payload")
		return
	}
	var n domain.Node
	var agentToken string
	var err error
	if req.NodeID != "" && req.EnrollmentToken != "" {
		n, agentToken, err = s.store.RegisterAgentWithEnrollment(r.Context(), req.NodeID, req.EnrollmentToken, req.Name, req.Address)
	} else if s.allowAutoRegister && s.authorizeAgentBootstrap(r, req.Token) {
		n, err = s.store.UpsertAgentNode(r.Context(), req.Name, req.Address, req.Token)
		agentToken = req.Token
	} else {
		writeErr(w, 401, "agent enrollment required: set MEGAVPN_AGENT_NODE_ID and MEGAVPN_AGENT_ENROLLMENT_TOKEN")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, response{"status": "registered", "node": n, "agent_token": agentToken, "token_hint": tokenHint(agentToken)})
}
func (s *Server) agentHeartbeat(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req agentHeartbeatReq
	if !decode(r, &req) {
		writeErr(w, 400, "invalid agent heartbeat payload")
		return
	}
	if !s.authorizeAgentNode(r, req.NodeID) {
		_ = s.store.RecordAgentAuthFailure(r.Context(), req.NodeID, "heartbeat unauthorized")
		writeErr(w, 401, "agent unauthorized")
		return
	}
	var err error
	if req.NodeID != "" {
		err = s.store.HeartbeatByNodeID(r.Context(), req.NodeID)
	} else {
		err = s.store.Heartbeat(r.Context(), req.Name)
	}
	if err != nil {
		writeErr(w, 404, "node not registered")
		return
	}
	writeJSON(w, 200, response{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
}
func (s *Server) agentInventory(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req agentInventoryReq
	if !decode(r, &req) || req.NodeID == "" || req.Inventory == nil {
		writeErr(w, 400, "invalid inventory payload")
		return
	}
	if !s.authorizeAgentNode(r, req.NodeID) {
		_ = s.store.RecordAgentAuthFailure(r.Context(), req.NodeID, "inventory unauthorized")
		writeErr(w, 401, "agent unauthorized")
		return
	}
	snap, caps, err := s.store.SubmitNodeInventory(r.Context(), req.NodeID, req.Inventory)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.store.RecordAgentInventorySync(r.Context(), req.NodeID, req.Source)
	writeJSON(w, 200, response{"status": "accepted", "snapshot": snap, "capabilities": caps})
}

func (s *Server) agentNextJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	nodeRef := r.URL.Query().Get("node_id")
	if nodeRef == "" {
		nodeRef = r.URL.Query().Get("node")
	}
	if !s.authorizeAgentNode(r, nodeRef) {
		_ = s.store.RecordAgentAuthFailure(r.Context(), nodeRef, "job poll unauthorized")
		writeErr(w, 401, "agent unauthorized")
		return
	}
	_ = s.store.TouchAgentJobPoll(r.Context(), nodeRef)
	j, ok, err := s.store.AgentNextJob(r.Context(), nodeRef)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		w.WriteHeader(204)
		return
	}
	if j.NodeID != nil {
		_ = s.store.RecordAgentJobClaim(r.Context(), *j.NodeID, j.ID, j.Type)
	}
	writeJSON(w, 200, j)
}
func (s *Server) agentJobResult(w nethttp.ResponseWriter, r *nethttp.Request) {
	idv := r.PathValue("id")
	if !s.authorizeAgentJob(r, idv) {
		if meta, err := s.store.GetJob(r.Context(), idv); err == nil && meta.NodeID != nil {
			_ = s.store.RecordAgentAuthFailure(r.Context(), *meta.NodeID, "job result unauthorized")
		}
		writeErr(w, 401, "agent unauthorized")
		return
	}
	var req agentResultReq
	if !decode(r, &req) || req.Status == "" {
		writeErr(w, 400, "invalid result payload")
		return
	}
	if req.Status != "succeeded" {
		req.Status = "failed"
	}
	jobMeta, _ := s.store.GetJob(r.Context(), idv)
	if err := s.store.CompleteJob(r.Context(), idv, req.Status, req.Result); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if jobMeta.NodeID != nil {
		_ = s.store.RecordAgentJobResult(r.Context(), *jobMeta.NodeID, idv, jobMeta.Type, req.Status)
	}
	_ = s.store.AddJobLog(r.Context(), idv, "info", "agent submitted result", req.Result)
	writeJSON(w, 200, response{"status": "accepted"})
}

func tokenHint(token string) string {
	if len(token) <= 14 {
		return token
	}
	return token[:8] + "..." + token[len(token)-6:]
}

func securityHeaders(enableHSTS bool, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if enableHSTS {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type lrw struct {
	nethttp.ResponseWriter
	code  int
	bytes int
}

func (w *lrw) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }
func (w *lrw) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}
func requestLogger(log *slog.Logger, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		start := time.Now()
		lw := &lrw{ResponseWriter: w, code: 200}
		next.ServeHTTP(lw, r)
		log.Info("http request", "method", r.Method, "path", r.URL.Path, "status", lw.code, "bytes", lw.bytes, "remote_addr", r.RemoteAddr, "duration_ms", time.Since(start).Milliseconds())
	})
}
func recoverer(log *slog.Logger, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		defer func() {
			if x := recover(); x != nil {
				log.Error("panic recovered", "panic", x, "path", r.URL.Path)
				writeErr(w, 500, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
