package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/rtis-emc2/megavpn/internal/platform/config"
	"github.com/rtis-emc2/megavpn/internal/platform/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)

	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := loadState(cfg.Agent.StatePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Error("agent state read failed", "path", cfg.Agent.StatePath, "error", err)
		os.Exit(1)
	}

	if st == nil {
		bootstrap, err := loadBootstrap(cfg)
		if err != nil {
			log.Error("agent bootstrap config invalid", "error", err)
			os.Exit(1)
		}
		st, err = enrollWithRetry(root, log, cfg, bootstrap)
		if err != nil {
			log.Error("agent enrollment stopped", "error", err)
			os.Exit(1)
		}
	} else {
		log.Info("agent state loaded", "node", st.NodeName, "node_id", st.NodeID, "state_path", cfg.Agent.StatePath, "version", appVersion)
	}

	runControlLoop(root, log, cfg.Agent.PollInterval, newClient(st.ControlPlaneURL, st.AgentToken, cfg.Agent.StatePath), st)
}
