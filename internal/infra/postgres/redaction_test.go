package postgres

import "testing"

func TestRedactSensitiveMapForStorageRemovesNestedDownloadToken(t *testing.T) {
	payload := map[string]any{
		"binary_repository": map[string]any{
			"download_token": "secret-ticket",
			"ticket_hint":    "abcd...wxyz",
		},
	}

	got := redactSensitiveMapForStorage(payload)
	repo, ok := got["binary_repository"].(map[string]any)
	if !ok {
		t.Fatalf("binary_repository type = %T", got["binary_repository"])
	}
	if repo["download_token"] != "[redacted]" {
		t.Fatalf("download_token was not redacted: %#v", repo)
	}
	if repo["ticket_hint"] != "abcd...wxyz" {
		t.Fatalf("ticket_hint should remain visible: %#v", repo)
	}
}
