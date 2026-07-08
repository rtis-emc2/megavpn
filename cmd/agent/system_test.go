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
