package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunSystemdDaemonReloadFailsClosed(t *testing.T) {
	oldRun := runInstallCommand
	defer func() { runInstallCommand = oldRun }()

	runInstallCommand = func(context.Context, string, ...string) (int, string) {
		return 7, "daemon reload denied\nsecond line"
	}

	result, err := runSystemdDaemonReload(context.Background())
	if err == nil {
		t.Fatal("expected daemon-reload failure")
	}
	if !strings.Contains(err.Error(), "daemon reload denied") {
		t.Fatalf("error = %q, want command output detail", err.Error())
	}
	if result["exit_code"] != 7 {
		t.Fatalf("exit_code = %#v, want 7", result["exit_code"])
	}
}

func TestRunSystemdDaemonReloadRecordsSuccessfulCommand(t *testing.T) {
	oldRun := runInstallCommand
	defer func() { runInstallCommand = oldRun }()

	runInstallCommand = func(context.Context, string, ...string) (int, string) {
		return 0, ""
	}

	result, err := runSystemdDaemonReload(context.Background())
	if err != nil {
		t.Fatalf("runSystemdDaemonReload returned error: %v", err)
	}
	if result["exit_code"] != 0 {
		t.Fatalf("exit_code = %#v, want 0", result["exit_code"])
	}
}

func TestWrapPackageManagerCommandUsesIsolatedTransientUnit(t *testing.T) {
	t.Parallel()

	name, args, ok := wrapPackageManagerCommand(
		"env",
		[]string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "-f", "install", "-y"},
		"agent-invocation",
		"/usr/bin/systemd-run",
		"megavpn-package-install-42-1",
	)
	if !ok {
		t.Fatal("expected package command to be wrapped")
	}
	if name != "/usr/bin/systemd-run" {
		t.Fatalf("wrapped command = %q", name)
	}
	got := strings.Join(args, " ")
	for _, want := range []string{
		"--system --quiet",
		"--wait --pipe --collect",
		"--unit megavpn-package-install-42-1",
		"--property=NoNewPrivileges=no",
		"--property=RestrictSUIDSGID=no",
		"--property=RuntimeMaxSec=9min",
		"-- env DEBIAN_FRONTEND=noninteractive apt-get -f install -y",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("wrapped args %q do not contain %q", got, want)
		}
	}
}

func TestWrapPackageManagerCommandDoesNotWrapOutsideSystemd(t *testing.T) {
	t.Parallel()

	if _, _, ok := wrapPackageManagerCommand("apt-get", []string{"update"}, "", "/usr/bin/systemd-run", "megavpn-package-install-42-2"); ok {
		t.Fatal("package command outside a systemd invocation must remain direct")
	}
}

func TestWrapPackageManagerCommandRejectsNonPackageCommand(t *testing.T) {
	t.Parallel()

	if _, _, ok := wrapPackageManagerCommand("xray", []string{"version"}, "agent-invocation", "/usr/bin/systemd-run", "megavpn-package-install-42-3"); ok {
		t.Fatal("non-package command must not be delegated")
	}
}

func TestAllowedPackageManagerCommand(t *testing.T) {
	t.Parallel()

	for _, command := range []struct {
		name string
		args []string
	}{
		{name: "apt-get", args: []string{"update"}},
		{name: "apt-get", args: []string{"-o", "Acquire::Retries=3", "-o", "Dpkg::Lock::Timeout=120", "update"}},
		{name: "dpkg", args: []string{"--configure", "-a"}},
		{name: "env", args: []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "ppp"}},
		{name: "env", args: []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "-f", "install", "-y"}},
		{name: "dpkg", args: []string{"-i", "/var/lib/megavpn/artifacts/runtime.deb"}},
		{name: "env", args: []string{"NEEDRESTART_MODE=a", "dpkg", "--configure", "-a"}},
	} {
		if !isAllowedPackageManagerCommand(command.name, command.args...) {
			t.Fatalf("expected %s %v to be classified as a package command", command.name, command.args)
		}
	}
	if isAllowedPackageManagerCommand("env", "FOO=bar", "xray", "version") {
		t.Fatal("env-wrapped non-package command was accepted")
	}
}

func TestWrapPackageManagerCommandRejectsUnsafeInvocationOrUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		unit string
	}{
		{name: "apt-get", args: []string{"download", "nginx"}, unit: "megavpn-package-install-42-4"},
		{name: "dpkg", args: []string{"--extract", "package.deb", "/tmp/out"}, unit: "megavpn-package-install-42-5"},
		{name: "env", args: []string{"DEBIAN_FRONTEND=noninteractive", "sh", "-c", "apt-get update"}, unit: "megavpn-package-install-42-6"},
		{name: "env", args: []string{"LD_PRELOAD=/tmp/evil.so", "apt-get", "update"}, unit: "megavpn-package-install-42-7"},
		{name: "apt-get", args: []string{"update"}, unit: "unsafe/unit"},
		{name: "apt-get", args: []string{"-o", "APT::Update::Pre-Invoke::=/tmp/hook", "update"}, unit: "megavpn-package-install-42-8"},
		{name: "apt-get", args: []string{"install", "-y", "./runtime.deb"}, unit: "megavpn-package-install-42-9"},
		{name: "dpkg", args: []string{"-i", "runtime.deb"}, unit: "megavpn-package-install-42-10"},
	}
	for _, tt := range tests {
		if _, _, ok := wrapPackageManagerCommand(tt.name, tt.args, "agent-invocation", "/usr/bin/systemd-run", tt.unit); ok {
			t.Fatalf("unsafe invocation was wrapped: %s %v unit=%q", tt.name, tt.args, tt.unit)
		}
	}
}
