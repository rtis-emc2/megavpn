package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func runInstallCommand(parent context.Context, name string, args ...string) (int, string) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	b, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(b)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(b)
	}
	return -1, err.Error() + "\n" + string(b)
}

func runOutput(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(b)
}

func runCombinedOutput(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	b, err := cmd.CombinedOutput()
	if err != nil && len(b) == 0 {
		return ""
	}
	return string(b)
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...<truncated>"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func normalizeSystemctlState(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "could not be found") || strings.Contains(s, "not-found") {
		return "unknown"
	}
	return s
}

func firstKnownUnit(names ...string) string {
	for _, name := range names {
		active := normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-active", name)))
		enabled := normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-enabled", name)))
		if active != "unknown" || enabled != "unknown" {
			return name
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func firstKnownActiveState(names ...string) string {
	for _, name := range names {
		active := strings.TrimSpace(runOutput("systemctl", "is-active", name))
		normalized := normalizeSystemctlState(active)
		if normalized != "unknown" {
			return normalized
		}
	}
	return "unknown"
}
