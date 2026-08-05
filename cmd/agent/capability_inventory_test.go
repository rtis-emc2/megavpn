package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type inventorySyncFailingDoer struct{}

func (inventorySyncFailingDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("control plane unavailable")
}

func TestRecordCapabilityInventorySyncPreservesFailureEvidence(t *testing.T) {
	t.Parallel()

	c := client{
		baseURL: "https://control.example.invalid",
		http:    inventorySyncFailingDoer{},
	}
	result := map[string]any{"ok": true, "message": "capability installed"}

	c.recordCapabilityInventorySync(context.Background(), "node-1", result)

	if got := stringFromMap(result, "inventory_sync_status"); got != "failed" {
		t.Fatalf("inventory_sync_status=%q, want failed", got)
	}
	if got := stringFromMap(result, "inventory_sync_error"); !strings.Contains(got, "control plane unavailable") {
		t.Fatalf("inventory_sync_error=%q, want transport evidence", got)
	}
	if result["ok"] != true {
		t.Fatalf("inventory sync failure must not rewrite capability outcome: %#v", result)
	}
}
