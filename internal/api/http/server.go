package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/jobschema"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

type Store interface {
	Ping(context.Context) error
	SchemaMigrationStatus(context.Context) (string, int, error)
	EnsureBootstrapPlatformUser(context.Context, string, string, string, string) (domain.PlatformUser, bool, error)
	GetPlatformUserForAuth(context.Context, string) (domain.PlatformUserAuth, error)
	GetPlatformUserByIDForAuth(context.Context, string) (domain.PlatformUserAuth, error)
	ListPlatformUsers(context.Context, int) ([]domain.PlatformUserRecord, error)
	CreatePlatformUser(context.Context, string, string, string, string, []string, *string) (domain.PlatformUserRecord, error)
	GetPlatformUserRecord(context.Context, string) (domain.PlatformUserRecord, error)
	GetPlatformMailSettings(context.Context) (domain.PlatformMailSettings, error)
	UpsertPlatformMailSettings(context.Context, domain.PlatformMailSettings, *string) (domain.PlatformMailSettings, error)
	MarkPlatformMailTest(context.Context, string) error
	GetControlPlaneTLSSettings(context.Context) (domain.ControlPlaneTLSSettings, error)
	UpsertControlPlaneTLSSettings(context.Context, domain.ControlPlaneTLSSettings, *string) (domain.ControlPlaneTLSSettings, error)
	CreateControlPlaneTLSApplyJob(context.Context) (domain.Job, error)
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
	CreateNodeEmergencyCleanupJob(context.Context, string, bool, string, string) (domain.Job, error)
	CreateNodeRoutePolicyApplyJob(context.Context, string) (domain.Job, error)
	RequeueNodeStuckJob(context.Context, string) (domain.Job, error)
	ClearNodeStalePendingRotation(context.Context, string) ([]domain.Job, error)
	RotateNodeEnrollmentToken(context.Context, string, time.Duration) (domain.NodeEnrollmentToken, error)
	RevokeNodeAgentIdentity(context.Context, string) (domain.Node, error)
	ListNodeAccessMethods(context.Context, string) ([]domain.NodeAccessMethod, error)
	ReplaceNodeAccessMethods(context.Context, string, []domain.NodeAccessMethod) ([]domain.NodeAccessMethod, error)
	CreateNodeBootstrapJob(context.Context, string, string, map[string]any) (domain.Job, domain.NodeBootstrapRun, error)
	ListNodeBootstrapRuns(context.Context, string, int) ([]domain.NodeBootstrapRun, error)
	LatestNodeInventory(context.Context, string) (domain.NodeInventorySnapshot, error)
	ListAllNodeCapabilities(context.Context) (map[string][]domain.NodeCapability, error)
	ListNodeCapabilities(context.Context, string) ([]domain.NodeCapability, error)
	ListNodeCapabilityInstallEvents(context.Context, string, int) ([]domain.NodeCapabilityInstallEvent, error)
	CreateNodeCapabilityInstallJob(context.Context, string, string, string, string) (domain.Job, error)
	CreateNodeCapabilityInstallJobWithDependents(context.Context, string, string, string, string, []string) (domain.Job, error)
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
	UpdateNode(context.Context, string, domain.Node) (domain.Node, error)
	UpdateNodeGeoIP(context.Context, string, domain.NodeGeoIP) (domain.Node, error)
	RetireNode(context.Context, string) (domain.Node, error)
	SetNodeMaintenance(context.Context, string, bool) (domain.Node, error)
	ListServiceDefinitions(context.Context) ([]domain.ServiceDefinition, error)
	EnsureDefaultVLESSGroupTemplates(context.Context, []domain.VLESSGroupTemplate) error
	ListVLESSGroupTemplates(context.Context) ([]domain.VLESSGroupTemplate, error)
	ListVLESSGroupTemplateCatalog(context.Context) ([]domain.VLESSGroupTemplate, error)
	UpsertVLESSGroupTemplate(context.Context, domain.VLESSGroupTemplate) (domain.VLESSGroupTemplate, error)
	SetVLESSGroupTemplateStatus(context.Context, string, string) (domain.VLESSGroupTemplate, error)
	ListBinaryArtifacts(context.Context, bool) ([]domain.BinaryArtifact, error)
	CreateBinaryArtifact(context.Context, domain.BinaryArtifact) (domain.BinaryArtifact, error)
	ResolveBinaryDownloadTicket(context.Context, string, string, string, string) (domain.BinaryDownloadTicket, domain.BinaryArtifact, error)
	MarkBinaryDownloadTicketUsed(context.Context, string, string) error
	ListInstances(context.Context) ([]domain.Instance, error)
	GetInstance(context.Context, string) (domain.Instance, error)
	GetInstanceWithSpec(context.Context, string) (domain.Instance, error)
	EnsureDefaultAddressPoolSpaces(context.Context) error
	AddressPoolInventory(context.Context) (domain.AddressPoolInventory, error)
	CreateAddressPoolSpace(context.Context, domain.AddressPoolSpace) (domain.AddressPoolSpace, error)
	UpdateAddressPoolSpace(context.Context, string, domain.AddressPoolSpace) (domain.AddressPoolSpace, error)
	DeleteAddressPoolSpace(context.Context, string) (domain.AddressPoolSpace, error)
	SetAddressPoolRouting(context.Context, string, bool) (domain.AddressPoolSpace, error)
	FirewallInventory(context.Context) (domain.FirewallInventory, error)
	CreateFirewallAddressList(context.Context, domain.FirewallAddressList) (domain.FirewallAddressList, error)
	UpdateFirewallAddressList(context.Context, string, domain.FirewallAddressList) (domain.FirewallAddressList, error)
	DeleteFirewallAddressList(context.Context, string) (domain.FirewallAddressList, error)
	CreateFirewallAddressEntry(context.Context, string, domain.FirewallAddressEntry) (domain.FirewallAddressEntry, error)
	UpdateFirewallAddressEntry(context.Context, string, string, domain.FirewallAddressEntry) (domain.FirewallAddressEntry, error)
	DeleteFirewallAddressEntry(context.Context, string, string) (domain.FirewallAddressEntry, error)
	CreateFirewallRule(context.Context, string, domain.FirewallRule) (domain.FirewallRule, error)
	UpdateFirewallRule(context.Context, string, string, domain.FirewallRule) (domain.FirewallRule, error)
	DeleteFirewallRule(context.Context, string, string) (domain.FirewallRule, error)
	CreateFirewallApplyJob(context.Context, string, string, bool) (domain.Job, error)
	ListInstanceRuntimeStates(context.Context) ([]domain.InstanceRuntimeState, error)
	GetInstanceRuntimeState(context.Context, string) (domain.InstanceRuntimeState, error)
	ListInstanceRuntimeObservations(context.Context, string, int) ([]domain.InstanceRuntimeObservation, error)
	ListAgentInstanceRuntimeTargets(context.Context, string) ([]domain.AgentInstanceRuntimeTarget, error)
	SubmitAgentInstanceRuntimeReports(context.Context, string, []domain.AgentInstanceRuntimeReport) ([]domain.InstanceRuntimeState, error)
	ListInstanceRevisions(context.Context, string, int) ([]domain.InstanceRevision, error)
	CreateInstance(context.Context, domain.Instance) (domain.Instance, error)
	CreateInstanceDraft(context.Context, domain.Instance) (domain.Instance, error)
	CreateInstanceValidatedDraft(context.Context, domain.Instance) (domain.Instance, error)
	DiscardInstanceDraft(context.Context, string) error
	ReplaceInstanceSpec(context.Context, string, string, map[string]any) (domain.InstanceRevision, error)
	RollbackInstanceRevision(context.Context, string, string, string) (domain.InstanceRevision, error)
	CreateInstanceDiagnosticsJob(context.Context, string) (domain.Job, error)
	UpdateInstanceStatus(context.Context, string, string) (domain.Job, error)
	DeleteInstance(context.Context, string) (domain.Instance, error)
	ListClients(context.Context) ([]domain.Client, error)
	GetClient(context.Context, string) (domain.Client, error)
	CreateClient(context.Context, domain.Client) (domain.Client, error)
	SetClientStatus(context.Context, string, string) (domain.Client, error)
	ProvisionClient(context.Context, string, []string) (domain.Job, error)
	ProvisionClientWithOptions(context.Context, string, []string, map[string]map[string]any) (domain.Job, error)
	CreateArtifactBuildJob(context.Context, string, string, []string) (domain.Job, error)
	RevokeClient(context.Context, string) (domain.Job, error)
	ListServiceAccesses(context.Context, string) ([]domain.ServiceAccess, error)
	ListClientAccessRoutes(context.Context, string) ([]domain.ClientAccessRoute, error)
	CreateClientAccessRoute(context.Context, string, domain.ClientAccessRoute) (domain.ClientAccessRoute, error)
	DeleteClientAccessRoute(context.Context, string, string) (domain.ClientAccessRoute, error)
	RotateServiceAccess(context.Context, string, string, string) (domain.Job, error)
	ListBackhaulLinks(context.Context) ([]domain.BackhaulLink, error)
	GetBackhaulLink(context.Context, string) (domain.BackhaulLink, error)
	CreateBackhaulLink(context.Context, domain.BackhaulLink) (domain.BackhaulLink, error)
	CreateBackhaulApplyJobs(context.Context, string) ([]domain.Job, error)
	CreateBackhaulProbeJobs(context.Context, string) ([]domain.Job, error)
	SetBackhaulRouteEnabled(context.Context, string, bool) (domain.BackhaulLink, []domain.Job, error)
	CreateBackhaulDeleteJobs(context.Context, string) (domain.BackhaulLink, []domain.Job, error)
	ListArtifacts(context.Context, string) ([]domain.Artifact, error)
	GetArtifact(context.Context, string, string) (domain.Artifact, error)
	PublishShareLink(context.Context, string, string, time.Duration) (domain.ShareLink, error)
	ListShareLinks(context.Context, string) ([]domain.ShareLink, error)
	RevokeShareLink(context.Context, string, string) (domain.ShareLink, error)
	ResolveShareLinkArtifact(context.Context, string) (domain.ShareLink, domain.Artifact, error)
	ListClientSubscriptions(context.Context, string) ([]domain.ClientSubscription, error)
	RotateClientSubscription(context.Context, string, time.Duration) (domain.ClientSubscription, error)
	RevokeClientSubscription(context.Context, string, string) (domain.ClientSubscription, error)
	ResolveClientVLESSSubscription(context.Context, string) (domain.ClientSubscriptionDocument, error)
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
	ListPlatformCertificates(context.Context) ([]domain.PlatformCertificate, error)
	GetPlatformCertificate(context.Context, string) (domain.PlatformCertificate, error)
	ImportPlatformCertificate(context.Context, string, string, []byte, []byte, []byte, bool) (domain.PlatformCertificate, error)
	CreateSelfSignedPlatformCertificate(context.Context, string, string, string, []string, int, bool) (domain.PlatformCertificate, error)
	CreateManagedPlatformCertificateAuthority(context.Context, string, string, string, int) (domain.PlatformCertificate, error)
	IssuePlatformCertificateFromAuthority(context.Context, string, string, string, string, []string, int, bool) (domain.PlatformCertificate, error)
	SetDefaultPlatformCertificate(context.Context, string) (domain.PlatformCertificateActionResult, error)
	RevokePlatformCertificate(context.Context, string) (domain.PlatformCertificateActionResult, error)
	DeletePlatformCertificateCascade(context.Context, string) (domain.PlatformCertificateActionResult, error)
	ListPlatformServicePKIRoots(context.Context) ([]domain.PlatformServicePKIRoot, error)
	CreateManagedPlatformServicePKIRoot(context.Context, string, string, string, int) (domain.PlatformServicePKIRoot, error)
	CreateNodeEnrollmentToken(context.Context, string, time.Duration) (domain.NodeEnrollmentToken, error)
	ListNodeEnrollmentTokens(context.Context, string) ([]domain.NodeEnrollmentToken, error)
	RegisterAgentWithEnrollmentVersion(context.Context, string, string, string, string, string, string) (domain.Node, string, error)
	UpsertAgentNodeWithVersion(context.Context, string, string, string, string, string) (domain.Node, error)
	ValidateAgentToken(context.Context, string, string) bool
	ValidateAgentTokenForJob(context.Context, string, string) bool
	RecordAgentAuthFailure(context.Context, string, string) error
	TouchAgentJobPoll(context.Context, string) error
	RecordAgentJobClaim(context.Context, string, string, string) error
	RecordAgentJobResult(context.Context, string, string, string, string) error
	RecordAgentInventorySync(context.Context, string, string) error
	HeartbeatByNodeIDWithVersion(context.Context, string, string, string) error
	HeartbeatWithVersion(context.Context, string, string, string) error
	AgentNextJob(context.Context, string) (domain.Job, bool, error)
	CompleteJob(context.Context, string, string, map[string]any) error
	CompleteAgentJob(context.Context, string, string, string, map[string]any) error
	AddJobLog(context.Context, string, string, string, map[string]any) error
}

type Server struct {
	log                    *slog.Logger
	store                  Store
	version                string
	listenAddr             string
	publicBaseURL          string
	productionMode         bool
	agentToken             string
	allowAutoRegister      bool
	agentSignatureEnforce  bool
	agentSignatureWindow   time.Duration
	agentSignatureReplay   *agentSignatureReplayCache
	sessionTTL             time.Duration
	sessionCookieName      string
	sessionCookieSecure    bool
	sessionCookieSecureSet bool
	webRoot                string
	rateLimiter            *rateLimiter
	terminalSessions       *terminalSessionStore
	trustProxyHeaders      bool
	maxRequestBytes        int64
	secretStorageReady     bool
	artifactRoot           string
	geoIPResolver          *nodeGeoIPResolver
	geoIPAutoEnrichLimit   int
}

type Options struct {
	Version                string
	ListenAddr             string
	PublicBaseURL          string
	ProductionMode         bool
	AgentToken             string
	AllowAutoRegister      bool
	AgentSignatureEnforce  bool
	AgentSignatureWindow   time.Duration
	SessionTTL             time.Duration
	SessionCookieName      string
	SessionCookieSecure    bool
	SessionCookieSecureSet bool
	WebRoot                string
	TrustProxyHeaders      bool
	MaxRequestBytes        int64
	SecretStorageReady     bool
	ArtifactRoot           string
	GeoIPLookupURLTemplate string
	GeoIPTimeout           time.Duration
	GeoIPAutoEnrichLimit   int
}

func New(log *slog.Logger, store Store, opts Options) nethttp.Handler {
	s := &Server{
		log:                    log,
		store:                  store,
		version:                opts.Version,
		listenAddr:             strings.TrimSpace(opts.ListenAddr),
		publicBaseURL:          strings.TrimRight(strings.TrimSpace(opts.PublicBaseURL), "/"),
		productionMode:         opts.ProductionMode,
		agentToken:             opts.AgentToken,
		allowAutoRegister:      opts.AllowAutoRegister,
		agentSignatureEnforce:  opts.AgentSignatureEnforce,
		agentSignatureWindow:   opts.AgentSignatureWindow,
		agentSignatureReplay:   newAgentSignatureReplayCache(opts.AgentSignatureWindow),
		sessionTTL:             opts.SessionTTL,
		sessionCookieName:      opts.SessionCookieName,
		sessionCookieSecure:    opts.SessionCookieSecure,
		sessionCookieSecureSet: opts.SessionCookieSecureSet,
		webRoot:                strings.TrimSpace(opts.WebRoot),
		rateLimiter:            newRateLimiter(),
		terminalSessions:       newTerminalSessionStore(),
		trustProxyHeaders:      opts.TrustProxyHeaders,
		maxRequestBytes:        opts.MaxRequestBytes,
		secretStorageReady:     opts.SecretStorageReady,
		artifactRoot:           strings.TrimSpace(opts.ArtifactRoot),
		geoIPResolver:          newNodeGeoIPResolver(opts.GeoIPLookupURLTemplate, opts.GeoIPTimeout),
		geoIPAutoEnrichLimit:   opts.GeoIPAutoEnrichLimit,
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
	mux.HandleFunc("GET /subscribe/vless/{token}", s.withRateLimit("public_vless_subscription", 120, time.Minute, s.publicVLESSSubscription))
	mux.HandleFunc("GET /assets/{path...}", s.assets)
	mux.HandleFunc("GET /agent/binary-artifacts/{artifact_id}/download", s.agentBinaryArtifactDownload)
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
	protected("GET /api/v1/settings/control-plane-tls", "settings.manage", s.getControlPlaneTLSSettings)
	protected("PUT /api/v1/settings/control-plane-tls", "settings.manage", s.updateControlPlaneTLSSettings)
	protected("POST /api/v1/settings/control-plane-tls/apply", "settings.manage", s.applyControlPlaneTLSSettings)
	protected("GET /api/v1/runtime/preflight", "settings.manage", s.runtimePreflight)
	protected("GET /api/v1/platform/certificates", "instance.read", s.listPlatformCertificates)
	protected("POST /api/v1/platform/certificates/preview", "instance.write", s.previewPlatformCertificate)
	protected("POST /api/v1/platform/certificates/import", "instance.write", s.importPlatformCertificate)
	protected("POST /api/v1/platform/certificates/self-signed", "instance.write", s.createSelfSignedPlatformCertificate)
	protected("POST /api/v1/platform/certificates/authorities", "instance.write", s.createManagedPlatformCertificateAuthority)
	protected("POST /api/v1/platform/certificates/issue-from-ca", "instance.write", s.issuePlatformCertificateFromAuthority)
	protected("POST /api/v1/platform/certificates/{id}/default", "instance.write", s.setDefaultPlatformCertificate)
	protected("POST /api/v1/platform/certificates/{id}/revoke", "instance.write", s.revokePlatformCertificate)
	protected("DELETE /api/v1/platform/certificates/{id}", "instance.write", s.deletePlatformCertificate)
	protected("GET /api/v1/platform/pki-roots", "instance.read", s.listPlatformServicePKIRoots)
	protected("POST /api/v1/platform/pki-roots", "instance.write", s.createManagedPlatformServicePKIRoot)
	protected("POST /api/v1/secret-refs", "node.bootstrap", s.createSecretRef)
	protected("GET /api/v1/dashboard", "dashboard.read", s.dashboard)
	protected("GET /api/v1/services", "service.read", s.listServices)
	protected("GET /api/v1/service-drivers", "service.read", s.listServiceDrivers)
	protected("GET /api/v1/services/installers", "service.read", s.listServiceInstallers)
	protected("GET /api/v1/service-packs", "service.read", s.listServicePacks)
	protected("PUT /api/v1/service-packs/{key}", "settings.manage", s.upsertServicePack)
	protected("POST /api/v1/service-packs/{key}/enable", "settings.manage", s.setServicePackStatus("active"))
	protected("POST /api/v1/service-packs/{key}/disable", "settings.manage", s.setServicePackStatus("disabled"))
	protected("DELETE /api/v1/service-packs/{key}", "settings.manage", s.setServicePackStatus("deleted"))
	protected("GET /api/v1/vless-groups", "service.read", s.listVLESSGroupTemplates)
	protected("PUT /api/v1/vless-groups/{key}", "settings.manage", s.upsertVLESSGroupTemplate)
	protected("POST /api/v1/vless-groups/{key}/enable", "settings.manage", s.setVLESSGroupTemplateStatus("active"))
	protected("POST /api/v1/vless-groups/{key}/disable", "settings.manage", s.setVLESSGroupTemplateStatus("disabled"))
	protected("DELETE /api/v1/vless-groups/{key}", "settings.manage", s.setVLESSGroupTemplateStatus("deleted"))
	protected("GET /api/v1/binary-artifacts", "binary_repository.read", s.listBinaryArtifacts)
	protected("POST /api/v1/binary-artifacts", "binary_repository.manage", s.createBinaryArtifact)
	protected("POST /api/v1/binary-artifacts/import", "binary_repository.manage", s.importBinaryArtifact)
	protected("POST /api/v1/binary-artifacts/import-url", "binary_repository.manage", s.importBinaryArtifactFromURL)
	protected("GET /api/v1/nodes", "node.read", s.listNodes)
	protected("POST /api/v1/nodes", "node.write", s.createNode)
	protected("GET /api/v1/nodes/{id}", "node.read", s.getNode)
	protected("PUT /api/v1/nodes/{id}", "node.write", s.updateNode)
	protected("GET /api/v1/nodes/{id}/diagnostics", "node.read", s.getNodeDiagnostics)
	protected("POST /api/v1/nodes/{id}/diagnostics/retry-inventory", "node.write", s.retryNodeInventorySync)
	protected("POST /api/v1/nodes/{id}/diagnostics/retry-discovery", "node.write", s.retryNodeDiscoverySync)
	protected("POST /api/v1/nodes/{id}/diagnostics/requeue-stuck-job", "node.write", s.requeueNodeStuckJob)
	protected("POST /api/v1/nodes/{id}/diagnostics/channel-probe", "node.write", s.probeNodeChannel)
	protected("POST /api/v1/nodes/{id}/routes/apply", "node.write", s.applyNodeRoutePolicy)
	protected("POST /api/v1/nodes/{id}/diagnostics/clear-stale-rotation", "node.bootstrap", s.clearNodeStaleRotation)
	protected("GET /api/v1/nodes/{id}/access-methods", "node.read", s.listNodeAccessMethods)
	protected("PUT /api/v1/nodes/{id}/access-methods", "node.bootstrap", s.replaceNodeAccessMethods)
	protected("POST /api/v1/nodes/{id}/ssh/sessions", "node.bootstrap", s.createNodeSSHTerminalSession)
	protected("GET /api/v1/nodes/{id}/ssh/terminal", "node.bootstrap", s.nodeSSHTerminal)
	protected("POST /api/v1/nodes/{id}/bootstrap", "node.bootstrap", s.createNodeBootstrapJob)
	protected("GET /api/v1/nodes/{id}/bootstrap-runs", "node.read", s.listNodeBootstrapRuns)
	protected("GET /api/v1/nodes/{id}/bootstrap-runs/{run_id}/bundle", "node.bootstrap", s.getNodeBootstrapBundle)
	protected("POST /api/v1/nodes/{id}/agent-token/rotate", "node.bootstrap", s.rotateNodeAgentToken)
	protected("POST /api/v1/nodes/{id}/enrollment-token/rotate", "node.bootstrap", s.rotateNodeEnrollmentToken)
	protected("POST /api/v1/nodes/{id}/agent-identity/revoke", "node.bootstrap", s.revokeNodeAgentIdentity)
	protected("POST /api/v1/nodes/{id}/emergency-cleanup", "node.bootstrap", s.createNodeEmergencyCleanupJob)
	protected("GET /api/v1/nodes/{id}/inventory", "node.read", s.getNodeInventory)
	protected("GET /api/v1/nodes/capabilities", "node.read", s.listAllNodeCapabilities)
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
	protected("GET /api/v1/instances/runtime-states", "instance.read", s.listInstanceRuntimeStates)
	protected("GET /api/v1/address-pools", "instance.read", s.listAddressPools)
	protected("POST /api/v1/address-pools/spaces", "settings.manage", s.createAddressPoolSpace)
	protected("PUT /api/v1/address-pools/spaces/{id}", "settings.manage", s.updateAddressPoolSpace)
	protected("DELETE /api/v1/address-pools/spaces/{id}", "settings.manage", s.deleteAddressPoolSpace)
	protected("POST /api/v1/address-pools/spaces/{id}/routing", "settings.manage", s.setAddressPoolRouting)
	protected("GET /api/v1/firewall", "firewall.read", s.listFirewallInventory)
	protected("POST /api/v1/firewall/policies", "firewall.manage", s.createFirewallPolicy)
	protected("PUT /api/v1/firewall/policies/{id}", "firewall.manage", s.updateFirewallPolicy)
	protected("DELETE /api/v1/firewall/policies/{id}", "firewall.manage", s.deleteFirewallPolicy)
	protected("POST /api/v1/firewall/address-lists", "firewall.manage", s.createFirewallAddressList)
	protected("PUT /api/v1/firewall/address-lists/{id}", "firewall.manage", s.updateFirewallAddressList)
	protected("DELETE /api/v1/firewall/address-lists/{id}", "firewall.manage", s.deleteFirewallAddressList)
	protected("POST /api/v1/firewall/address-lists/{id}/entries", "firewall.manage", s.createFirewallAddressEntry)
	protected("PUT /api/v1/firewall/address-lists/{id}/entries/{entry_id}", "firewall.manage", s.updateFirewallAddressEntry)
	protected("DELETE /api/v1/firewall/address-lists/{id}/entries/{entry_id}", "firewall.manage", s.deleteFirewallAddressEntry)
	protected("POST /api/v1/firewall/policies/{id}/rules", "firewall.manage", s.createFirewallRule)
	protected("PUT /api/v1/firewall/policies/{id}/rules/{rule_id}", "firewall.manage", s.updateFirewallRule)
	protected("DELETE /api/v1/firewall/policies/{id}/rules/{rule_id}", "firewall.manage", s.deleteFirewallRule)
	protected("POST /api/v1/nodes/{id}/firewall/apply", "firewall.apply", s.applyNodeFirewallPolicy)
	protected("POST /api/v1/instances", "instance.write", s.createInstance)
	protected("POST /api/v1/service-packs/{key}/instances", "instance.write", s.createServicePackInstances)
	protected("GET /api/v1/instances/{id}", "instance.read", s.getInstance)
	protected("GET /api/v1/instances/{id}/runtime-state", "instance.read", s.getInstanceRuntimeState)
	protected("GET /api/v1/instances/{id}/runtime-observations", "instance.read", s.listInstanceRuntimeObservations)
	protected("GET /api/v1/instances/{id}/revisions", "instance.read", s.listInstanceRevisions)
	protected("PUT /api/v1/instances/{id}/spec", "instance.write", s.replaceInstanceSpec)
	protected("POST /api/v1/instances/{id}/rollback", "instance.write", s.rollbackInstanceRevision)
	protected("DELETE /api/v1/instances/{id}", "instance.write", s.deleteInstance)
	protected("POST /api/v1/instances/{id}/apply", "instance.apply", s.instanceAction("apply"))
	protected("POST /api/v1/instances/{id}/restart", "instance.apply", s.instanceAction("restart"))
	protected("POST /api/v1/instances/{id}/start", "instance.apply", s.instanceAction("start"))
	protected("POST /api/v1/instances/{id}/stop", "instance.apply", s.instanceAction("stop"))
	protected("POST /api/v1/instances/{id}/enable", "instance.apply", s.instanceAction("enable"))
	protected("POST /api/v1/instances/{id}/disable", "instance.apply", s.instanceAction("disable"))
	protected("POST /api/v1/instances/{id}/diagnose", "instance.read", s.diagnoseInstance)
	protected("GET /api/v1/clients", "client.read", s.listClients)
	protected("POST /api/v1/clients", "client.write", s.createClient)
	protected("GET /api/v1/clients/{id}", "client.read", s.getClient)
	protected("DELETE /api/v1/clients/{id}", "client.write", s.deleteClient)
	protected("POST /api/v1/clients/{id}/provision", "client.provision", s.provisionClient)
	protected("POST /api/v1/clients/{id}/revoke", "client.provision", s.revokeClient)
	protected("POST /api/v1/clients/{id}/suspend", "client.write", s.clientStatus("suspended"))
	protected("POST /api/v1/clients/{id}/activate", "client.write", s.clientStatus("active"))
	protected("GET /api/v1/clients/{id}/accesses", "client.read", s.clientAccesses)
	protected("GET /api/v1/clients/{id}/routes", "client.read", s.clientAccessRoutes)
	protected("POST /api/v1/clients/{id}/routes", "client.provision", s.createClientAccessRoute)
	protected("DELETE /api/v1/clients/{id}/routes/{route_id}", "client.provision", s.deleteClientAccessRoute)
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-openvpn", "client.provision", s.rotateClientAccess("openvpn"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-xray", "client.provision", s.rotateClientAccess("xray-core"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-wireguard", "client.provision", s.rotateClientAccess("wireguard"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-mtproto", "client.provision", s.rotateClientAccess("mtproto"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-ipsec", "client.provision", s.rotateClientAccess("ipsec"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-http-proxy", "client.provision", s.rotateClientAccess("http_proxy"))
	protected("POST /api/v1/clients/{id}/accesses/{access_id}/rotate-shadowsocks", "client.provision", s.rotateClientAccess("shadowsocks"))
	protected("GET /api/v1/clients/{id}/artifacts", "artifact.read", s.clientArtifacts)
	protected("POST /api/v1/clients/{id}/artifacts", "artifact.export", s.createArtifact)
	protected("GET /api/v1/clients/{id}/artifacts/{artifact_id}/content", "artifact.read", s.clientArtifactContent)
	protected("GET /api/v1/clients/{id}/artifacts/{artifact_id}/download", "artifact.read", s.clientArtifactDownload)
	protected("GET /api/v1/clients/{id}/share-links", "artifact.read", s.clientShareLinks)
	protected("POST /api/v1/clients/{id}/share-links", "share_link.manage", s.publishShareLink)
	protected("POST /api/v1/clients/{id}/share-links/{link_id}/revoke", "share_link.manage", s.revokeShareLink)
	protected("GET /api/v1/clients/{id}/subscriptions", "client.read", s.clientSubscriptions)
	protected("POST /api/v1/clients/{id}/subscriptions/rotate", "client.provision", s.rotateClientSubscription)
	protected("POST /api/v1/clients/{id}/subscriptions/{subscription_id}/revoke", "client.provision", s.revokeClientSubscription)
	protected("POST /api/v1/clients/{id}/deliver-email", "artifact.export", s.deliverClientEmail)
	protected("GET /api/v1/backhaul/drivers", "node.read", s.listBackhaulDrivers)
	protected("GET /api/v1/backhaul-links", "node.read", s.listBackhaulLinks)
	protected("POST /api/v1/backhaul-links", "node.write", s.createBackhaulLink)
	protected("GET /api/v1/backhaul-links/{id}", "node.read", s.getBackhaulLink)
	protected("POST /api/v1/backhaul-links/{id}/apply", "node.write", s.applyBackhaulLink)
	protected("POST /api/v1/backhaul-links/{id}/probe", "node.write", s.probeBackhaulLink)
	protected("PATCH /api/v1/backhaul-links/{id}/route", "node.write", s.updateBackhaulRouteState)
	protected("DELETE /api/v1/backhaul-links/{id}", "node.write", s.deleteBackhaulLink)
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
	mux.HandleFunc("GET /agent/runtime/instances", s.agentRuntimeTargets)
	mux.HandleFunc("POST /agent/runtime/instances", s.agentRuntimeReport)
	mux.HandleFunc("GET /agent/jobs/next", s.agentNextJob)
	mux.HandleFunc("POST /agent/jobs/{id}/result", s.agentJobResult)
	handler := nethttp.Handler(mux)
	if s.maxRequestBytes > 0 {
		handler = maxRequestBytesHandler(handler, s.maxRequestBytes)
	}
	return recoverer(log, requestLogger(log, securityHeaders(strings.HasPrefix(s.publicBaseURL, "https://"), handler)))
}

func maxRequestBytesHandler(next nethttp.Handler, defaultLimit int64) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		limit := defaultLimit
		if r.Method == nethttp.MethodPost && r.URL != nil && r.URL.Path == "/api/v1/binary-artifacts/import" {
			artifactLimit := maxBinaryArtifactUploadBytes + 1024*1024
			if artifactLimit > limit {
				limit = artifactLimit
			}
		}
		r.Body = nethttp.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

type response map[string]any

type provisionRequest struct {
	InstanceIDs    []string                  `json:"instance_ids"`
	ServiceOptions map[string]map[string]any `json:"service_options"`
}
type artifactRequest struct {
	Type        string   `json:"type"`
	InstanceIDs []string `json:"instance_ids"`
}
type instanceSpecRequest struct {
	Spec map[string]any `json:"spec"`
}
type rollbackRevisionRequest struct {
	RevisionID string `json:"revision_id"`
}
type shareLinkRequest struct {
	TargetID string `json:"target_id"`
	TTLHours int    `json:"ttl_hours"`
}
type subscriptionRequest struct {
	TTLHours int `json:"ttl_hours"`
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

type nodeEmergencyCleanupRequest struct {
	IncludeAgent bool   `json:"include_agent"`
	Confirmation string `json:"confirmation"`
	CleanupScope string `json:"cleanup_scope"`
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
	body, err := requestBodySnapshot(r)
	if err != nil {
		return false
	}
	if optional && len(body) == 0 {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(body))
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
	setWebAssetNoStore(w)
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
	setWebAssetNoStore(w)
	if s.serveFileIfExists(w, r, filepath.Join("assets", assetPath), "") {
		return
	}
	nethttp.NotFound(w, r)
}

func setWebAssetNoStore(w nethttp.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
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
	if s.productionMode {
		checks := s.runtimePreflightChecks(ctx)
		status := runtimePreflightStatus(checks)
		code := 200
		readiness := "ready"
		if !runtimePreflightIsReady(checks) {
			code = 503
			readiness = "not_ready"
		}
		writeJSON(w, code, response{
			"status":           readiness,
			"service":          "megavpn-api",
			"version":          s.version,
			"production_mode":  true,
			"preflight_status": status,
			"checks":           checks,
			"time":             time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	if err := s.store.Ping(ctx); err != nil {
		writeErr(w, 503, "database is not ready")
		return
	}
	writeJSON(w, 200, response{"status": "ready", "service": "megavpn-api", "version": s.version, "production_mode": false, "time": time.Now().UTC().Format(time.RFC3339)})
}
func (s *Server) versionHandler(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, response{
		"service":                "megavpn-api",
		"version":                s.version,
		"agent_target_version":   s.version,
		"agent_protocol_version": "v1",
		"public_base_url":        s.publicBaseURL,
		"agent_register_url":     joinPublicURL(s.publicBaseURL, "/agent/register"),
		"agent_heartbeat_url":    joinPublicURL(s.publicBaseURL, "/agent/heartbeat"),
		"agent_next_job_url":     joinPublicURL(s.publicBaseURL, "/agent/jobs/next"),
		"agent_job_result_url":   joinPublicURL(s.publicBaseURL, "/agent/jobs/{id}/result"),
		"agent_inventory_url":    joinPublicURL(s.publicBaseURL, "/agent/inventory"),
		"agent_runtime_url":      joinPublicURL(s.publicBaseURL, "/agent/runtime/instances"),
		"agent_public_url_note":  "MEGAVPN_PUBLIC_BASE_URL must be reachable from every remote agent node, including custom ports.",
		"time":                   time.Now().UTC().Format(time.RFC3339),
	})
}

func joinPublicURL(baseURL, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + path
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
	writeJSON(w, 200, enrichServiceDefinitions(x))
}

func (s *Server) listServiceDrivers(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, driver.Contracts())
}

func (s *Server) listServiceInstallers(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, []response{
		{"service_code": "nginx", "strategy": "nginx_org_repo", "channel": "stable", "description": "Install nginx from the official nginx.org Ubuntu repository; falls back to ubuntu_repo when CPU ISA is incompatible."},
		{"service_code": "nginx", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install nginx from the Ubuntu repository."},
		{"service_code": "nginx", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed nginx."},
		{"service_code": "xray-core", "strategy": "binary_repository", "channel": "stable", "description": "Install Xray-core from a pinned control-plane binary repository artifact."},
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
		{"service_code": "shadowsocks", "strategy": "binary_repository", "channel": "stable", "description": "Install a pinned ss-server binary from the control-plane binary repository."},
		{"service_code": "shadowsocks", "strategy": "ubuntu_repo", "channel": "stable", "description": "Install shadowsocks-libev from the Ubuntu repository; this provides the ss-server runtime used by managed instances."},
		{"service_code": "shadowsocks", "strategy": "manual_present", "channel": "none", "description": "Verify and register an already installed ss-server runtime."},
	})
}

func (s *Server) listNodes(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListNodes(r.Context())
	if err != nil {
		writeErr(w, 500, "list nodes failed")
		return
	}
	x = s.enrichNodeGeoIP(r.Context(), x)
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
	writeJSON(w, 202, response{"job": redactedJob(job), "bootstrap_run": redactedBootstrapRun(run)})
}

func (s *Server) createNodeEmergencyCleanupJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeEmergencyCleanupRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid emergency cleanup payload")
		return
	}
	job, err := s.store.CreateNodeEmergencyCleanupJob(r.Context(), idParam(r), req.IncludeAgent, req.Confirmation, req.CleanupScope)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(job))
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
	writeJSON(w, 200, redactedBootstrapRuns(x))
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

func (s *Server) listAllNodeCapabilities(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListAllNodeCapabilities(r.Context())
	if err != nil {
		writeErr(w, 500, "list node capabilities failed")
		return
	}
	if x == nil {
		x = map[string][]domain.NodeCapability{}
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
	writeJSON(w, 202, redactedJob(j))
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
	writeJSON(w, 202, redactedJob(j))
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
	writeJSON(w, 200, redactedNodeCapabilityInstallEvents(x))
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
	writeJSON(w, 202, redactedJob(x))
}

func (s *Server) createNodeInventoryJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	j, err := s.store.CreateNodeInventoryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(j))
}

func (s *Server) createNode(w nethttp.ResponseWriter, r *nethttp.Request) {
	var n domain.Node
	if !decode(r, &n) || n.Name == "" || n.Address == "" {
		writeErr(w, 400, "invalid node payload: name and address are required")
		return
	}
	if err := validateNodeProfile(n); err != "" {
		writeErr(w, 400, err)
		return
	}
	s.resolveNodeGeoIPForProfile(r.Context(), &n)
	x, err := s.store.CreateNode(r.Context(), n)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, x)
}

func (s *Server) updateNode(w nethttp.ResponseWriter, r *nethttp.Request) {
	var n domain.Node
	if !decode(r, &n) || n.Name == "" || n.Address == "" {
		writeErr(w, 400, "invalid node payload: name and address are required")
		return
	}
	if err := validateNodeProfile(n); err != "" {
		writeErr(w, 400, err)
		return
	}
	s.resolveNodeGeoIPForProfile(r.Context(), &n)
	x, err := s.store.UpdateNode(r.Context(), idParam(r), n)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func validateNodeProfile(n domain.Node) string {
	if err := domain.ValidateRequiredSingleLine("node name", n.Name); err != nil {
		return "invalid node payload: " + err.Error()
	}
	if err := domain.ValidateRequiredSingleLine("node address", n.Address); err != nil {
		return "invalid node payload: " + err.Error()
	}
	if n.Role != "" && n.Role != "ingress" && n.Role != "egress" {
		return "invalid node payload: role must be ingress or egress"
	}
	if n.Kind != "" && n.Kind != "local" && n.Kind != "remote" {
		return "invalid node payload: kind must be local or remote"
	}
	switch n.ExecutionMode {
	case "", "agent_managed", "ssh_bootstrap", "manual_bundle", "local_managed":
		return ""
	default:
		return "invalid node payload: execution_mode is not supported"
	}
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
	if x == nil {
		x = []domain.NodeEnrollmentToken{}
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

func (s *Server) listInstanceRuntimeStates(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListInstanceRuntimeStates(r.Context())
	if err != nil {
		writeErr(w, 500, "list instance runtime states failed")
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

func (s *Server) getInstanceRuntimeState(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetInstanceRuntimeState(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "instance runtime state not found")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) listInstanceRuntimeObservations(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListInstanceRuntimeObservations(r.Context(), idParam(r), limit)
	if err != nil {
		writeErr(w, 500, "list instance runtime observations failed")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) listInstanceRevisions(w nethttp.ResponseWriter, r *nethttp.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	instanceID := idParam(r)
	x, err := s.store.ListInstanceRevisions(r.Context(), instanceID, limit)
	if err != nil {
		if s.log != nil {
			s.log.Error(
				"list instance revisions failed",
				"instance_id", instanceID,
				"limit", limit,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"err", err,
			)
		}
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
	message := "instance revision saved as apply-ready"
	if revision.Status != "validated" && revision.Status != "applied" {
		message = "instance revision saved with validation issues"
	}
	writeJSON(w, 200, response{
		"revision":    revision,
		"can_apply":   revision.Status == "validated" || revision.Status == "applied",
		"message":     message,
		"issue_count": len(revision.ValidationErrors),
	})
}

func (s *Server) rollbackInstanceRevision(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req rollbackRevisionRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid rollback payload")
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
	revision, err := s.store.RollbackInstanceRevision(r.Context(), idParam(r), strings.TrimSpace(req.RevisionID), source)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	instanceID := idParam(r)
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "instance.revision.rollback", "instance", &instanceID, "instance revision rollback created")
	message := "rollback revision created and is apply-ready"
	if revision.Status != "validated" && revision.Status != "applied" {
		message = "rollback revision created with validation issues"
	}
	writeJSON(w, 200, response{
		"revision":    revision,
		"can_apply":   revision.Status == "validated" || revision.Status == "applied",
		"message":     message,
		"issue_count": len(revision.ValidationErrors),
	})
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
func (s *Server) diagnoseInstance(w nethttp.ResponseWriter, r *nethttp.Request) {
	j, err := s.store.CreateInstanceDiagnosticsJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(j))
}
func (s *Server) instanceAction(action string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		j, err := s.store.UpdateInstanceStatus(r.Context(), idParam(r), action)
		if err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		writeJSON(w, 202, redactedJob(j))
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
	j, err := s.store.ProvisionClientWithOptions(r.Context(), idParam(r), req.InstanceIDs, req.ServiceOptions)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(j))
}
func (s *Server) revokeClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	j, err := s.store.RevokeClient(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(j))
}
func (s *Server) clientAccesses(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListServiceAccesses(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list accesses failed")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) clientAccessRoutes(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListClientAccessRoutes(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list client routes failed")
		return
	}
	if x == nil {
		x = []domain.ClientAccessRoute{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) createClientAccessRoute(w nethttp.ResponseWriter, r *nethttp.Request) {
	var route domain.ClientAccessRoute
	if !decode(r, &route) {
		writeErr(w, 400, "invalid route payload")
		return
	}
	x, err := s.store.CreateClientAccessRoute(r.Context(), idParam(r), route)
	if err != nil {
		writeErr(w, classifyClientRouteErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, x)
}

func (s *Server) deleteClientAccessRoute(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.DeleteClientAccessRoute(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("route_id")))
	if err != nil {
		writeErr(w, classifyClientRouteErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func classifyClientRouteErrStatus(err error) int {
	switch {
	case err == nil:
		return 500
	case strings.Contains(err.Error(), "required"),
		strings.Contains(err.Error(), "unsupported"),
		strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "must be"),
		strings.Contains(err.Error(), "must target"):
		return 400
	case strings.Contains(err.Error(), "not found"):
		return 404
	default:
		return 409
	}
}

func (s *Server) rotateClientAccess(driver string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		jobRecord, err := s.store.RotateServiceAccess(r.Context(), idParam(r), r.PathValue("access_id"), driver)
		if err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		writeJSON(w, 202, redactedJob(jobRecord))
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
	jobRecord, err := s.store.CreateArtifactBuildJob(r.Context(), idParam(r), req.Type, req.InstanceIDs)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"job":            redactedJob(jobRecord),
		"requested_type": req.Type,
		"message":        "artifact build queued",
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
func (s *Server) clientSubscriptions(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.ListClientSubscriptions(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 500, "list client subscriptions failed")
		return
	}
	if x == nil {
		x = []domain.ClientSubscription{}
	}
	writeJSON(w, 200, x)
}
func (s *Server) rotateClientSubscription(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req subscriptionRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid subscription payload")
		return
	}
	if req.TTLHours < 0 || req.TTLHours > 8760 {
		writeErr(w, 400, "ttl_hours must be between 0 and 8760")
		return
	}
	ttl := time.Duration(req.TTLHours) * time.Hour
	x, err := s.store.RotateClientSubscription(r.Context(), idParam(r), ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	redacted := x
	redacted.Token = ""
	writeJSON(w, 201, response{
		"subscription":     redacted,
		"subscription_url": joinPublicURL(s.publicBaseURL, "/subscribe/vless/"+x.Token),
		"message":          "VLESS subscription token rotated; copy the URL now because the token is not stored in plaintext.",
	})
}
func (s *Server) revokeClientSubscription(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.RevokeClientSubscription(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("subscription_id")))
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
	writeJSON(w, 200, redactedJobs(x))
}
func (s *Server) createJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	var j domain.Job
	if !decode(r, &j) || j.Type == "" {
		writeErr(w, 400, "invalid job payload: type is required")
		return
	}
	if jobTypeMustUseTypedEndpoint(j.Type) {
		writeErr(w, 400, "privileged job type must be created through its typed API")
		return
	}
	if !jobTypeAllowedInGenericAPI(j.Type) {
		writeErr(w, 400, "job type is not available through generic job API")
		return
	}
	if permission := requiredPermissionForJobType(j.Type); permission != "" {
		authCtx, ok := authFromRequest(r)
		if !ok || !authContextHasPermission(authCtx, permission) {
			writeErr(w, 403, "job type requires "+permission)
			return
		}
	}
	if j.ScopeType == "" {
		j.ScopeType = "system"
	}
	j.Status = "queued"
	x, err := s.store.CreateJob(r.Context(), j)
	if err != nil {
		if jobschema.IsValidationError(err) {
			writeErr(w, 400, err.Error())
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(x))
}

func (s *Server) getJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "job not found")
		return
	}
	writeJSON(w, 200, redactedJob(x))
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
	writeJSON(w, 200, redactedJob(x))
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
	AgentVersion    string `json:"agent_version"`
	ProtocolVersion string `json:"protocol_version"`
}
type agentHeartbeatReq struct {
	NodeID          string `json:"node_id"`
	Name            string `json:"name"`
	AgentVersion    string `json:"agent_version"`
	ProtocolVersion string `json:"protocol_version"`
}
type agentInventoryReq struct {
	NodeID    string         `json:"node_id"`
	Source    string         `json:"source"`
	Inventory map[string]any `json:"inventory"`
}
type agentRuntimeReportReq struct {
	NodeID  string                              `json:"node_id"`
	Reports []domain.AgentInstanceRuntimeReport `json:"reports"`
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
		return s.verifyAgentSignature(r, "node:"+strings.TrimSpace(nodeID), tok)
	}
	if s.allowAutoRegister && s.agentToken != "" && tok == s.agentToken {
		return s.verifyAgentSignature(r, "auto-node:"+strings.TrimSpace(nodeID), tok)
	}
	return false
}

func (s *Server) authorizeAgentJob(r *nethttp.Request, jobID string) bool {
	tok := bearerToken(r)
	if tok == "" || jobID == "" {
		return false
	}
	if s.store != nil && s.store.ValidateAgentTokenForJob(r.Context(), jobID, tok) {
		return s.verifyAgentSignature(r, "job:"+strings.TrimSpace(jobID), tok)
	}
	if s.allowAutoRegister && s.agentToken != "" && tok == s.agentToken {
		return s.verifyAgentSignature(r, "auto-job:"+strings.TrimSpace(jobID), tok)
	}
	return false
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
		n, agentToken, err = s.store.RegisterAgentWithEnrollmentVersion(r.Context(), req.NodeID, req.EnrollmentToken, req.Name, req.Address, req.AgentVersion, req.ProtocolVersion)
	} else if s.allowAutoRegister && s.authorizeAgentBootstrap(r, req.Token) {
		n, err = s.store.UpsertAgentNodeWithVersion(r.Context(), req.Name, req.Address, req.Token, req.AgentVersion, req.ProtocolVersion)
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
		err = s.store.HeartbeatByNodeIDWithVersion(r.Context(), req.NodeID, req.AgentVersion, req.ProtocolVersion)
	} else {
		err = s.store.HeartbeatWithVersion(r.Context(), req.Name, req.AgentVersion, req.ProtocolVersion)
	}
	if err != nil {
		writeErr(w, 404, "node not registered")
		return
	}
	writeSignedAgentJSON(w, r, bearerToken(r), 200, response{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
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
	writeSignedAgentJSON(w, r, bearerToken(r), 200, response{"status": "accepted", "snapshot": snap, "capabilities": caps})
}

func (s *Server) agentRuntimeTargets(w nethttp.ResponseWriter, r *nethttp.Request) {
	nodeRef := strings.TrimSpace(r.URL.Query().Get("node_id"))
	if nodeRef == "" {
		nodeRef = strings.TrimSpace(r.URL.Query().Get("node"))
	}
	if nodeRef == "" {
		writeErr(w, 400, "node_id is required")
		return
	}
	if !s.authorizeAgentNode(r, nodeRef) {
		_ = s.store.RecordAgentAuthFailure(r.Context(), nodeRef, "runtime targets unauthorized")
		writeErr(w, 401, "agent unauthorized")
		return
	}
	targets, err := s.store.ListAgentInstanceRuntimeTargets(r.Context(), nodeRef)
	if err != nil {
		s.log.Error("agent runtime targets failed", "node_id", nodeRef, "error", err)
		writeErr(w, 500, "list runtime targets failed")
		return
	}
	writeSignedAgentJSON(w, r, bearerToken(r), 200, response{"targets": targets})
}

func (s *Server) agentRuntimeReport(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req agentRuntimeReportReq
	if !decode(r, &req) || strings.TrimSpace(req.NodeID) == "" {
		writeErr(w, 400, "invalid runtime report payload")
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	if !s.authorizeAgentNode(r, req.NodeID) {
		_ = s.store.RecordAgentAuthFailure(r.Context(), req.NodeID, "runtime report unauthorized")
		writeErr(w, 401, "agent unauthorized")
		return
	}
	states, err := s.store.SubmitAgentInstanceRuntimeReports(r.Context(), req.NodeID, req.Reports)
	if err != nil {
		s.log.Error("agent runtime report failed", "node_id", req.NodeID, "reports", len(req.Reports), "error", err)
		writeErr(w, 500, "submit runtime report failed")
		return
	}
	writeSignedAgentJSON(w, r, bearerToken(r), 200, response{"status": "accepted", "states": states})
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
		writeSignedAgentNoContent(w, r, bearerToken(r))
		return
	}
	if j.NodeID != nil {
		_ = s.store.RecordAgentJobClaim(r.Context(), *j.NodeID, j.ID, j.Type)
	}
	writeSignedAgentJSON(w, r, bearerToken(r), 200, j)
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
	owner := ""
	if jobMeta.NodeID != nil {
		owner = "agent:" + strings.TrimSpace(*jobMeta.NodeID)
	}
	if err := s.store.CompleteAgentJob(r.Context(), idv, owner, req.Status, req.Result); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if jobMeta.NodeID != nil {
		_ = s.store.RecordAgentJobResult(r.Context(), *jobMeta.NodeID, idv, jobMeta.Type, req.Status)
	}
	_ = s.store.AddJobLog(r.Context(), idv, "info", "agent submitted result", req.Result)
	writeSignedAgentJSON(w, r, bearerToken(r), 200, response{"status": "accepted"})
}

func jobTypeMustUseTypedEndpoint(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case "platform.control_plane_tls.apply",
		"node.bootstrap",
		"node.agent.rotate_token",
		"node.emergency_cleanup",
		"node.backhaul.apply",
		"node.backhaul.probe",
		"node.backhaul.cleanup",
		"node.route_policy.apply",
		"node.firewall.preview",
		"node.firewall.apply",
		"node.firewall.observe",
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

func jobTypeAllowedInGenericAPI(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case "artifact.build", "client.provision", "client.revoke":
		return true
	default:
		return false
	}
}

func requiredPermissionForJobType(jobType string) string {
	switch strings.TrimSpace(jobType) {
	case "platform.control_plane_tls.apply":
		return "settings.manage"
	case "node.bootstrap", "node.agent.rotate_token", "node.emergency_cleanup":
		return "node.bootstrap"
	case "node.capability.install", "node.capability.verify", "node.inventory", "node.inventory.sync", "node.services.discover", "node.channel.probe", "node.backhaul.apply", "node.backhaul.probe", "node.backhaul.cleanup", "node.route_policy.apply":
		return "node.write"
	case "node.firewall.preview", "node.firewall.apply", "node.firewall.observe":
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
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
func (w *lrw) Unwrap() nethttp.ResponseWriter { return w.ResponseWriter }
func (w *lrw) Flush() {
	if flusher, ok := w.ResponseWriter.(nethttp.Flusher); ok {
		flusher.Flush()
	}
}
func (w *lrw) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(nethttp.Hijacker)
	if !ok {
		return nil, nil, errors.New("http hijacker is not supported")
	}
	return hijacker.Hijack()
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
