package main

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func TestSystemdArgsForOperation(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		driver.OperationRestart: {"restart", "megavpn-test"},
		driver.OperationStart:   {"start", "megavpn-test"},
		driver.OperationStop:    {"stop", "megavpn-test"},
		driver.OperationEnable:  {"enable", "--now", "megavpn-test"},
		driver.OperationDisable: {"disable", "--now", "megavpn-test"},
	}
	for operation, want := range cases {
		operation, want := operation, want
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			got, err := systemdArgsForOperation(operation, "megavpn-test")
			if err != nil {
				t.Fatalf("systemdArgsForOperation returned error: %v", err)
			}
			if len(got) != len(want) {
				t.Fatalf("args len = %d, want %d: %#v", len(got), len(want), got)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("args[%d] = %q, want %q; all args=%#v", i, got[i], want[i], got)
				}
			}
		})
	}
}

func TestSystemdArgsForUnsupportedOperation(t *testing.T) {
	t.Parallel()

	if _, err := systemdArgsForOperation("destroy", "megavpn-test"); err == nil {
		t.Fatal("expected unsupported operation error")
	}
}
