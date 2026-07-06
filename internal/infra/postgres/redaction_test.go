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

func TestRedactSensitiveMapForStorageRemovesRuntimeSecrets(t *testing.T) {
	payload := map[string]any{
		"files": []any{
			map[string]any{"path": "/etc/wireguard/wg0.conf", "content": "PrivateKey = very-secret"},
		},
		"settings": map[string]any{
			"privateKey":   "xray-reality-secret",
			"content_hash": "safe-hash",
			"bundle":       "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
		},
	}

	got := redactSensitiveMapForStorage(payload)
	files := got["files"].([]any)
	file := files[0].(map[string]any)
	if file["content"] != "[redacted]" {
		t.Fatalf("file content was not redacted: %#v", file)
	}
	settings := got["settings"].(map[string]any)
	if settings["privateKey"] != "[redacted]" {
		t.Fatalf("camel-case privateKey was not redacted: %#v", settings)
	}
	if settings["bundle"] != "[redacted]" {
		t.Fatalf("PEM private key content was not redacted: %#v", settings)
	}
	if settings["content_hash"] != "safe-hash" {
		t.Fatalf("content_hash should remain visible: %#v", settings)
	}
}
