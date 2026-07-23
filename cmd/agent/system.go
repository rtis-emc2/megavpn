package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

var runInstallCommand = runInstallCommandDefault

var packageInstallUnitSequence atomic.Uint64

func runInstallCommandDefault(parent context.Context, name string, args ...string) (int, string) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	execName, execArgs := name, args
	packageUnit := ""
	if systemdRunPath, err := exec.LookPath("systemd-run"); err == nil {
		packageUnit = fmt.Sprintf("megavpn-package-install-%d-%d", os.Getpid(), packageInstallUnitSequence.Add(1))
		if wrappedName, wrappedArgs, ok := wrapPackageManagerCommand(name, args, os.Getenv("INVOCATION_ID"), systemdRunPath, packageUnit); ok {
			execName, execArgs = wrappedName, wrappedArgs
		} else {
			packageUnit = ""
		}
	}
	cmd := exec.CommandContext(ctx, execName, execArgs...)
	b, err := cmd.CombinedOutput()
	output := string(b)
	if packageUnit != "" {
		output = "MegaVPN package helper unit: " + packageUnit + "\n" + output
	}
	if err == nil {
		return 0, output
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), output
	}
	return -1, err.Error() + "\n" + output
}

func wrapPackageManagerCommand(name string, args []string, invocationID, systemdRunPath, unitName string) (string, []string, bool) {
	if strings.TrimSpace(invocationID) == "" || strings.TrimSpace(systemdRunPath) == "" || strings.TrimSpace(unitName) == "" {
		return name, args, false
	}
	if !isAllowedPackageManagerCommand(name, args...) || !isSafeSystemdUnitName(unitName) {
		return name, args, false
	}
	wrapped := []string{
		"--system",
		"--quiet",
		"--wait",
		"--pipe",
		"--collect",
		"--service-type=exec",
		"--unit", unitName,
		"--property=NoNewPrivileges=no",
		"--property=RestrictSUIDSGID=no",
		"--property=PrivateTmp=no",
		"--property=RuntimeMaxSec=9min",
		"--setenv=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"--",
		name,
	}
	wrapped = append(wrapped, args...)
	return systemdRunPath, wrapped, true
}

func isAllowedPackageManagerCommand(name string, args ...string) bool {
	manager, managerArgs, ok := packageManagerInvocation(name, args)
	if !ok {
		return false
	}
	switch manager {
	case "apt-get":
		return isAllowedAptGetCommand(managerArgs)
	case "dpkg":
		if len(managerArgs) == 0 {
			return false
		}
		switch managerArgs[0] {
		case "-i", "--install":
			return len(managerArgs) == 2 && strings.HasPrefix(managerArgs[1], "/") && !strings.ContainsRune(managerArgs[1], 0)
		case "--configure":
			return len(managerArgs) == 2 && (managerArgs[1] == "-a" || managerArgs[1] == "--pending")
		}
	}
	return false
}

func packageManagerInvocation(name string, args []string) (string, []string, bool) {
	name = strings.TrimSpace(name)
	if name == "apt-get" || name == "dpkg" {
		return name, args, true
	}
	if name != "env" {
		return "", nil, false
	}
	for index, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		if strings.Contains(arg, "=") {
			if !isAllowedPackageManagerEnvironment(arg) {
				return "", nil, false
			}
			continue
		}
		if arg != "apt-get" && arg != "dpkg" {
			return "", nil, false
		}
		return arg, args[index+1:], true
	}
	return "", nil, false
}

func isAllowedPackageManagerEnvironment(value string) bool {
	switch value {
	case "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1":
		return true
	default:
		return false
	}
}

func isAllowedAptGetCommand(args []string) bool {
	action := ""
	fixBroken := false
	packages := 0
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "" {
			return false
		}
		if arg == "-o" || arg == "--option" {
			if index+1 >= len(args) || !isAllowedAptGetOption(strings.TrimSpace(args[index+1])) {
				return false
			}
			index++
			continue
		}
		switch arg {
		case "-f", "--fix-broken":
			fixBroken = true
			continue
		case "-y", "--assume-yes":
			continue
		}
		if action == "" {
			if arg != "update" && arg != "install" && arg != "purge" {
				return false
			}
			action = arg
			continue
		}
		if action == "update" || !isSafeDebianPackageName(arg) {
			return false
		}
		packages++
	}
	if action == "update" {
		return true
	}
	return action != "" && (packages > 0 || (action == "install" && fixBroken))
}

func isAllowedAptGetOption(value string) bool {
	switch value {
	case "APT::Sandbox::User=root",
		"Acquire::Retries=3",
		"Dpkg::Lock::Timeout=120",
		"Dpkg::Options::=--force-confdef",
		"Dpkg::Options::=--force-confold":
		return true
	default:
		return false
	}
}

func isSafeDebianPackageName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		if index > 0 && (r == '+' || r == '-' || r == '.' || r == ':') {
			continue
		}
		return false
	}
	return true
}

func isSafeSystemdUnitName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '@' {
			continue
		}
		return false
	}
	return true
}

func runSystemdDaemonReload(parent context.Context) (map[string]any, error) {
	code, out := runInstallCommand(parent, "systemctl", "daemon-reload")
	result := map[string]any{
		"command":   []string{"systemctl", "daemon-reload"},
		"exit_code": code,
		"output":    truncate(strings.TrimSpace(out), 2000),
	}
	if code != 0 {
		return result, commandExitError("systemctl daemon-reload failed", out)
	}
	return result, nil
}

func commandExitError(prefix, output string) error {
	if detail := firstLine(output); detail != "" {
		return fmt.Errorf("%s: %s", prefix, detail)
	}
	return fmt.Errorf("%s", prefix)
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
