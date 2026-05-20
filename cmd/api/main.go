package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	apihttp "github.com/rtis-emc2/megavpn/internal/api/http"
	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
	"github.com/rtis-emc2/megavpn/internal/platform/database"
	"github.com/rtis-emc2/megavpn/internal/platform/logger"
	"github.com/rtis-emc2/megavpn/internal/platform/version"
	"github.com/rtis-emc2/megavpn/internal/secrets"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.Database.DSN)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	store := postgres.New(db.Pool)
	_ = store.SeedLocalInventory(context.Background())
	if strings.TrimSpace(cfg.Secrets.MasterKeyPath) != "" {
		secretSvc, err := secrets.LoadFromFile(cfg.Secrets.MasterKeyPath, cfg.Secrets.MasterKeyVersion)
		if err != nil {
			log.Error("master key load failed", "path", cfg.Secrets.MasterKeyPath, "error", err)
			os.Exit(1)
		}
		store.SetSecretService(secretSvc)
		log.Info("master key loaded", "key_version", secretSvc.KeyVersion(), "path", cfg.Secrets.MasterKeyPath)
	} else {
		log.Warn("master key path is not configured; secret storage endpoints should remain disabled until configured")
	}
	if username := strings.TrimSpace(cfg.Auth.BootstrapAdminUsername); username != "" && cfg.Auth.BootstrapAdminPassword != "" {
		hash, err := authn.HashPassword(cfg.Auth.BootstrapAdminPassword)
		if err != nil {
			log.Error("bootstrap admin password hash failed", "error", err)
			os.Exit(1)
		}
		user, created, err := store.EnsureBootstrapPlatformUser(context.Background(), username, cfg.Auth.BootstrapAdminEmail, cfg.Auth.BootstrapAdminDisplayName, hash)
		if err != nil {
			log.Error("bootstrap admin ensure failed", "username", username, "error", err)
			os.Exit(1)
		}
		if created {
			log.Info("bootstrap admin created", "username", user.Username, "user_id", user.ID)
		}
	} else if cfg.Auth.BootstrapAdminUsername != "" || cfg.Auth.BootstrapAdminPassword != "" {
		log.Warn("bootstrap admin config is incomplete; set both MEGAVPN_BOOTSTRAP_ADMIN_USERNAME and MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD")
	}
	webRoot := resolveWebRoot(cfg.API.WebRoot)
	if webRoot == "" {
		log.Warn("web ui root was not found; API fallback page will be served at /")
	} else {
		log.Info("web ui root resolved", "path", webRoot)
	}
	srv := &http.Server{
		Addr: cfg.API.ListenAddr,
		Handler: apihttp.New(log, store, apihttp.Options{
			Version:             version.Version,
			PublicBaseURL:       cfg.API.PublicBaseURL,
			AgentToken:          cfg.Agent.Token,
			AllowAutoRegister:   cfg.Agent.AllowAutoRegister,
			SessionTTL:          cfg.Auth.SessionTTL,
			SessionCookieName:   cfg.Auth.SessionCookieName,
			SessionCookieSecure: cfg.Auth.SessionCookieSecure,
			WebRoot:             webRoot,
			TrustProxyHeaders:   cfg.API.TrustProxyHeaders,
			MaxRequestBytes:     cfg.API.MaxRequestBytes,
		}),
		ReadTimeout:  cfg.API.ReadTimeout,
		WriteTimeout: cfg.API.WriteTimeout,
		IdleTimeout:  cfg.API.IdleTimeout,
	}
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ch := make(chan error, 1)
	go func() {
		log.Info("starting api server", "listen_addr", cfg.API.ListenAddr, "version", version.Version)
		ch <- srv.ListenAndServe()
	}()
	select {
	case err := <-ch:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api failed", "error", err)
			os.Exit(1)
		}
	case <-root.Done():
		log.Info("shutdown signal received")
	}
	shutdown(root, log, srv, cfg.API.ShutdownTimeout)
}
func shutdown(_ context.Context, log *slog.Logger, srv *http.Server, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		_ = srv.Close()
		os.Exit(1)
	}
	log.Info("api server stopped")
}

func resolveWebRoot(configured string) string {
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

	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
