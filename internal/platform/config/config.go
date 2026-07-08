package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	API       APIConfig
	Agent     AgentConfig
	Artifacts ArtifactConfig
	Auth      AuthConfig
	GeoIP     GeoIPConfig
	Secrets   SecretsConfig
	Worker    WorkerConfig
	Database  DatabaseConfig
	LogLevel  string
}

type APIConfig struct {
	ListenAddr              string
	PublicBaseURL           string
	ProductionMode          bool
	WebRoot                 string
	TrustProxyHeaders       bool
	MaxRequestBytes         int64
	ReadTimeout             time.Duration
	WriteTimeout            time.Duration
	IdleTimeout             time.Duration
	ShutdownTimeout         time.Duration
	FirewallSourceCIDRs     []string
	SSHBootstrapSourceCIDRs []string
}

type AgentConfig struct {
	NodeName          string
	NodeAddress       string
	ControlPlaneURL   string
	Token             string
	NodeID            string
	EnrollmentToken   string
	AllowAutoRegister bool
	SignatureEnforce  bool
	SignatureWindow   time.Duration
	PollInterval      time.Duration
	StatePath         string
	BootstrapPath     string
}

type ArtifactConfig struct {
	Root string
}

type GeoIPConfig struct {
	LookupURLTemplate string
	Timeout           time.Duration
	AutoEnrichLimit   int
}

type AuthConfig struct {
	SessionTTL                time.Duration
	SessionCookieName         string
	SessionCookieSecure       bool
	SessionCookieSecureSet    bool
	BootstrapAdminUsername    string
	BootstrapAdminEmail       string
	BootstrapAdminPassword    string
	BootstrapAdminDisplayName string
}

type SecretsConfig struct {
	MasterKeyPath    string
	MasterKeyVersion string
}

type WorkerConfig struct {
	Interval      time.Duration
	LeaseDuration time.Duration
	WorkerID      string
}

type DatabaseConfig struct{ DSN string }

func Load() Config {
	publicBaseURL := getEnv("MEGAVPN_PUBLIC_BASE_URL", "http://127.0.0.1:8080")
	return Config{
		API: APIConfig{
			ListenAddr:              getEnv("MEGAVPN_API_LISTEN_ADDR", "0.0.0.0:8080"),
			PublicBaseURL:           publicBaseURL,
			ProductionMode:          getEnvBool("MEGAVPN_PRODUCTION_MODE", false),
			WebRoot:                 strings.TrimSpace(getEnv("MEGAVPN_WEB_ROOT", "")),
			TrustProxyHeaders:       getEnvBool("MEGAVPN_TRUST_PROXY_HEADERS", false),
			MaxRequestBytes:         getEnvInt64("MEGAVPN_API_MAX_REQUEST_BYTES", 16*1024*1024),
			ReadTimeout:             getEnvDuration("MEGAVPN_API_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:            getEnvDuration("MEGAVPN_API_WRITE_TIMEOUT", 20*time.Second),
			IdleTimeout:             getEnvDuration("MEGAVPN_API_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:         getEnvDuration("MEGAVPN_API_SHUTDOWN_TIMEOUT", 10*time.Second),
			FirewallSourceCIDRs:     getEnvList("MEGAVPN_CP_FIREWALL_SOURCE_CIDRS"),
			SSHBootstrapSourceCIDRs: getEnvList("MEGAVPN_CP_SSH_BOOTSTRAP_SOURCE_CIDRS"),
		},
		Agent: AgentConfig{
			NodeName:          getEnv("MEGAVPN_AGENT_NODE_NAME", hostname()),
			NodeAddress:       getEnv("MEGAVPN_AGENT_NODE_ADDRESS", "127.0.0.1"),
			ControlPlaneURL:   getEnv("MEGAVPN_AGENT_CONTROL_PLANE_URL", "http://127.0.0.1:8080"),
			Token:             getEnv("MEGAVPN_AGENT_TOKEN", ""),
			NodeID:            getEnv("MEGAVPN_AGENT_NODE_ID", ""),
			EnrollmentToken:   getEnv("MEGAVPN_AGENT_ENROLLMENT_TOKEN", ""),
			AllowAutoRegister: getEnvBool("MEGAVPN_AGENT_ALLOW_AUTO_REGISTER", false),
			SignatureEnforce:  getEnvBool("MEGAVPN_AGENT_SIGNATURE_ENFORCE", true),
			SignatureWindow:   getEnvDuration("MEGAVPN_AGENT_SIGNATURE_WINDOW", 5*time.Minute),
			PollInterval:      getEnvDuration("MEGAVPN_AGENT_POLL_INTERVAL", 10*time.Second),
			StatePath:         getEnv("MEGAVPN_AGENT_STATE_PATH", "/var/lib/megavpn/agent/state.json"),
			BootstrapPath:     getEnv("MEGAVPN_AGENT_BOOTSTRAP_PATH", "/etc/megavpn/agent-bootstrap.env"),
		},
		Artifacts: ArtifactConfig{
			Root: strings.TrimSpace(getEnv("MEGAVPN_ARTIFACT_ROOT", "/var/lib/megavpn/artifacts")),
		},
		GeoIP: GeoIPConfig{
			LookupURLTemplate: strings.TrimSpace(getEnv("MEGAVPN_GEOIP_LOOKUP_URL_TEMPLATE", "https://ipapi.co/{ip}/json/")),
			Timeout:           getEnvDuration("MEGAVPN_GEOIP_TIMEOUT", 3*time.Second),
			AutoEnrichLimit:   getEnvInt("MEGAVPN_GEOIP_AUTO_ENRICH_LIMIT", 5),
		},
		Auth: AuthConfig{
			SessionTTL:                getEnvDuration("MEGAVPN_AUTH_SESSION_TTL", 24*time.Hour),
			SessionCookieName:         getEnv("MEGAVPN_AUTH_SESSION_COOKIE_NAME", "megavpn_session"),
			SessionCookieSecure:       getEnvBool("MEGAVPN_AUTH_SESSION_COOKIE_SECURE", strings.HasPrefix(strings.ToLower(publicBaseURL), "https://")),
			SessionCookieSecureSet:    envIsSet("MEGAVPN_AUTH_SESSION_COOKIE_SECURE"),
			BootstrapAdminUsername:    strings.ToLower(strings.TrimSpace(getEnv("MEGAVPN_BOOTSTRAP_ADMIN_USERNAME", ""))),
			BootstrapAdminEmail:       strings.ToLower(strings.TrimSpace(getEnv("MEGAVPN_BOOTSTRAP_ADMIN_EMAIL", "superadmin@rtis.local"))),
			BootstrapAdminPassword:    getEnv("MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD", ""),
			BootstrapAdminDisplayName: strings.TrimSpace(getEnv("MEGAVPN_BOOTSTRAP_ADMIN_DISPLAY_NAME", "Superadmin")),
		},
		Secrets: SecretsConfig{
			MasterKeyPath:    strings.TrimSpace(getEnv("MEGAVPN_MASTER_KEY_PATH", "")),
			MasterKeyVersion: strings.TrimSpace(getEnv("MEGAVPN_MASTER_KEY_VERSION", "v1")),
		},
		Worker: WorkerConfig{
			Interval:      getEnvDuration("MEGAVPN_WORKER_INTERVAL", 3*time.Second),
			LeaseDuration: getEnvDuration("MEGAVPN_WORKER_LEASE_DURATION", 2*time.Minute),
			WorkerID:      getEnv("MEGAVPN_WORKER_ID", hostname()+"-worker"),
		},
		Database: DatabaseConfig{DSN: getEnv("MEGAVPN_DATABASE_DSN", getEnv("MEGAVPN_DATABASE_URL", ""))},
		LogLevel: getEnv("MEGAVPN_LOG_LEVEL", "info"),
	}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "local"
	}
	return h
}
func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envIsSet(k string) bool {
	_, ok := os.LookupEnv(k)
	return ok
}

func getEnvDuration(k string, d time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	if x, err := time.ParseDuration(v); err == nil {
		return x
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return d
}

func getEnvBool(k string, d bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	if v == "" {
		return d
	}
	return v == "1" || v == "true" || v == "yes" || v == "y" || v == "on"
}

func getEnvInt64(k string, d int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return d
	}
	return n
}

func getEnvInt(k string, d int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return d
	}
	return n
}

func getEnvList(k string) []string {
	raw := strings.TrimSpace(os.Getenv(k))
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
