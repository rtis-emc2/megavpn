package postgres

import (
	"strings"
	"testing"
)

func TestNormalizeEditableClientStringRejectsTooLongValues(t *testing.T) {
	value := strings.Repeat("x", 201)
	if _, err := normalizeEditableClientString(&value, 200, "display_name"); err == nil {
		t.Fatal("expected too-long display_name to be rejected")
	}
}

func TestMaskDeliveryEmail(t *testing.T) {
	tests := map[string]string{
		"alpha@example.test": "a***@example.test",
		"@example.test":      "***@example.test",
		"not-an-email":       "masked recipient",
		"":                   "internal delivery",
	}
	for input, want := range tests {
		if got := maskDeliveryEmail(input); got != want {
			t.Fatalf("maskDeliveryEmail(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSafeDeliveryErrorSummaryRedactsSensitiveValues(t *testing.T) {
	got := safeDeliveryErrorSummary("smtp rejected password=super-secret")
	if got != "delivery failed; sensitive error details are redacted" {
		t.Fatalf("expected sensitive error redaction, got %q", got)
	}
	if got := safeDeliveryErrorSummary("temporary smtp failure\nretry later"); got != "temporary smtp failure retry later" {
		t.Fatalf("expected newline-normalized safe error, got %q", got)
	}
}
