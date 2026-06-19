package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

func loadState(path string) (*agentState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st agentState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st.NodeID == "" || st.AgentToken == "" || st.ControlPlaneURL == "" {
		return nil, errors.New("agent state is incomplete")
	}
	return &st, nil
}

func saveState(path string, st agentState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func removeBootstrapFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) > 0 {
		zero := make([]byte, len(b))
		_ = os.WriteFile(path, zero, 0o600)
	}
	return os.Remove(path)
}

func loadBootstrap(cfg config.Config) (bootstrapConfig, error) {
	b := bootstrapConfig{
		NodeID:          cfg.Agent.NodeID,
		NodeName:        cfg.Agent.NodeName,
		NodeAddress:     cfg.Agent.NodeAddress,
		ControlPlaneURL: cfg.Agent.ControlPlaneURL,
		EnrollmentToken: cfg.Agent.EnrollmentToken,
		DevToken:        cfg.Agent.Token,
		AllowAuto:       cfg.Agent.AllowAutoRegister,
	}
	if fileCfg, err := readEnvFile(cfg.Agent.BootstrapPath); err == nil {
		b.NodeID = first(fileCfg["MEGAVPN_AGENT_NODE_ID"], b.NodeID)
		b.NodeName = first(fileCfg["MEGAVPN_AGENT_NODE_NAME"], b.NodeName)
		b.NodeAddress = first(fileCfg["MEGAVPN_AGENT_NODE_ADDRESS"], b.NodeAddress)
		b.ControlPlaneURL = first(fileCfg["MEGAVPN_AGENT_CONTROL_PLANE_URL"], b.ControlPlaneURL)
		b.EnrollmentToken = first(fileCfg["MEGAVPN_AGENT_ENROLLMENT_TOKEN"], b.EnrollmentToken)
		b.DevToken = first(fileCfg["MEGAVPN_AGENT_TOKEN"], b.DevToken)
	}
	if b.ControlPlaneURL == "" {
		b.ControlPlaneURL = "http://127.0.0.1:8080"
	}
	if b.NodeName == "" {
		b.NodeName = "unknown"
	}
	if b.NodeAddress == "" {
		b.NodeAddress = "127.0.0.1"
	}
	if b.NodeID == "" || b.EnrollmentToken == "" {
		if b.AllowAuto && b.DevToken != "" {
			return b, nil
		}
		return b, fmt.Errorf("first enrollment requires MEGAVPN_AGENT_NODE_ID and MEGAVPN_AGENT_ENROLLMENT_TOKEN in %s or environment", cfg.Agent.BootstrapPath)
	}
	return b, nil
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), "\"")
		if k != "" {
			out[k] = v
		}
	}
	return out, s.Err()
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
