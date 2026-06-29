package main

import (
	"os"
	"os/exec"
	"strings"
)

func findExecutable(name string, candidates ...string) string {
	path, _ := resolveExecutable(name, candidates...)
	return path
}

func resolveExecutable(name string, candidates ...string) (string, bool) {
	name = strings.TrimSpace(name)
	if name != "" {
		if path, err := exec.LookPath(name); err == nil && path != "" {
			return path, true
		}
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}
	return name, false
}

func xrayExecutablePath() string {
	return findExecutable("xray", "/usr/local/bin/xray", "/usr/bin/xray", "/opt/xray/xray")
}
