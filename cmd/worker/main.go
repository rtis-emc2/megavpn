package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
	"github.com/rtis-emc2/megavpn/internal/platform/database"
	"github.com/rtis-emc2/megavpn/internal/platform/logger"
	platformversion "github.com/rtis-emc2/megavpn/internal/platform/version"
	"github.com/rtis-emc2/megavpn/internal/secrets"
)

const binaryDownloadTicketRetention = 7 * 24 * time.Hour

func main() {
	if platformversion.CommandRequested(os.Args[1:]) {
		fmt.Println(platformversion.Version)
		return
	}

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
	store.SetArtifactRoot(cfg.Artifacts.Root)
	if strings.TrimSpace(cfg.Secrets.MasterKeyPath) != "" {
		secretSvc, err := secrets.LoadFromFile(cfg.Secrets.MasterKeyPath, cfg.Secrets.MasterKeyVersion)
		if err != nil {
			log.Error("master key load failed", "path", cfg.Secrets.MasterKeyPath, "error", err)
			os.Exit(1)
		}
		store.SetSecretService(secretSvc)
		log.Info("master key loaded", "key_version", secretSvc.KeyVersion(), "path", cfg.Secrets.MasterKeyPath)
	} else {
		log.Warn("master key path is not configured; secret-backed provisioning will stay disabled")
	}
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(cfg.Worker.Interval)
	defer ticker.Stop()
	log.Info("starting worker", "worker_id", cfg.Worker.WorkerID, "version", platformversion.Version)
	for {
		select {
		case <-root.Done():
			log.Info("worker stopped")
			return
		case <-ticker.C:
			runOnce(root, log, store, cfg)
		}
	}
}

func runOnce(ctx context.Context, log interface {
	Info(string, ...any)
	Error(string, ...any)
}, store *postgres.Store, cfg config.Config) {
	workerID := cfg.Worker.WorkerID
	expiredTickets, deletedTickets, err := store.CleanupBinaryDownloadTickets(ctx, binaryDownloadTicketRetention)
	if err != nil {
		log.Error("binary download ticket cleanup failed", "error", err)
	} else if expiredTickets > 0 || deletedTickets > 0 {
		log.Info("binary download ticket cleanup completed", "expired", expiredTickets, "deleted", deletedTickets)
	}
	job, ok, err := store.ClaimJob(ctx, workerID)
	if err != nil {
		log.Error("claim job failed", "error", err)
		return
	}
	if !ok {
		return
	}
	_ = store.AddJobLog(ctx, job.ID, "info", "worker started job", map[string]any{"type": job.Type})
	result := map[string]any{"handled_by": "worker", "type": job.Type}
	status := "succeeded"
	switch job.Type {
	case "platform.control_plane_tls.apply":
		status, result = handleControlPlaneTLSApplyJob(ctx, store, job)
	case "node.bootstrap":
		status, result = handleNodeBootstrapJob(ctx, log, store, cfg, job)
	case "client.provision":
		status, result = handleClientProvisionJob(ctx, store, job)
	case "artifact.build":
		status, result = handleClientProvisionJob(ctx, store, job)
	case "node.backhaul.apply", "node.backhaul.probe", "node.backhaul.cleanup":
		status = "failed"
		result["error"] = job.Type + " must be handled by an agent"
	case "instance.apply", "instance.restart", "instance.start", "instance.stop", "instance.enable", "instance.disable", "instance.delete":
		status = "failed"
		result["error"] = job.Type + " must be handled by an agent"
	default:
		status = "failed"
		result["error"] = "unsupported worker job type"
	}
	if err := store.CompleteJob(ctx, job.ID, status, result); err != nil {
		log.Error("complete job failed", "job_id", job.ID, "error", err)
		return
	}
	_ = store.AddJobLog(ctx, job.ID, "info", "worker finished job", result)
	log.Info("job completed", "job_id", job.ID, "type", job.Type)
}
