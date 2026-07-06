package http

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestParseTrafficAccountingExportTime(t *testing.T) {
	got, err := parseTrafficAccountingExportTime("2026-07-06T12:34:56Z")
	if err != nil {
		t.Fatalf("parse RFC3339 time: %v", err)
	}
	if got == nil || got.UTC().Format(time.RFC3339) != "2026-07-06T12:34:56Z" {
		t.Fatalf("parsed time = %v, want 2026-07-06T12:34:56Z", got)
	}
	got, err = parseTrafficAccountingExportTime("2026-07-06")
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	if got == nil || got.UTC().Format(time.RFC3339) != "2026-07-06T00:00:00Z" {
		t.Fatalf("parsed date = %v, want UTC midnight", got)
	}
	if _, err := parseTrafficAccountingExportTime("06.07.2026"); err == nil {
		t.Fatal("expected invalid time format to fail")
	}
}

func TestTrafficAccountingExportFilterFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/traffic/accounting/export?limit=123&from=2026-07-01&to=2026-07-06T23:59:59Z&client_id=00000000-0000-0000-0000-000000000001&node_id=00000000-0000-0000-0000-000000000002&protocol=wireguard", nil)
	filter, err := trafficAccountingExportFilterFromRequest(req)
	if err != nil {
		t.Fatalf("parse export filter: %v", err)
	}
	if filter.Limit != 123 || filter.Protocol != "wireguard" {
		t.Fatalf("unexpected filter scalar fields: %#v", filter)
	}
	if filter.ClientAccountID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("client id = %q", filter.ClientAccountID)
	}
	if filter.NodeID != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("node id = %q", filter.NodeID)
	}
	if filter.From == nil || filter.From.UTC().Format(time.RFC3339) != "2026-07-01T00:00:00Z" {
		t.Fatalf("from = %v", filter.From)
	}
	if filter.To == nil || filter.To.UTC().Format(time.RFC3339) != "2026-07-06T23:59:59Z" {
		t.Fatalf("to = %v", filter.To)
	}
}

func TestTrafficAccountingExportFilterRejectsInvertedRange(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/traffic/accounting/export?from=2026-07-06&to=2026-07-01", nil)
	if _, err := trafficAccountingExportFilterFromRequest(req); err == nil {
		t.Fatal("expected inverted range to fail")
	}
}

func TestTrafficAccountingCSVRowEscapesMetadata(t *testing.T) {
	start := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	row := trafficAccountingCSVRow(domain.TrafficAccountingSample{
		ID:             "sample-1",
		NodeID:         "node-1",
		NodeName:       "node,one",
		ClientUsername: "client-a",
		Source:         "agent",
		Protocol:       "wireguard",
		Direction:      "bidirectional",
		BucketStart:    start,
		BucketEnd:      start.Add(time.Minute),
		RxBytes:        128,
		TxBytes:        512,
		Metadata: map[string]any{
			"collector": "wireguard_transfer",
			"note":      "contains,comma",
		},
		ObservedAt: start.Add(time.Minute),
		ReceivedAt: start.Add(2 * time.Minute),
	})
	if len(row) != len(trafficAccountingCSVHeader()) {
		t.Fatalf("row len = %d, header len = %d", len(row), len(trafficAccountingCSVHeader()))
	}
	if row[2] != "node,one" || row[9] != "wireguard" || row[13] != "128" || row[14] != "512" {
		t.Fatalf("unexpected row = %#v", row)
	}
	if !strings.Contains(row[len(row)-1], `"collector":"wireguard_transfer"`) {
		t.Fatalf("metadata json missing collector: %q", row[len(row)-1])
	}
}

func TestTrafficAccountingCSVHeaderHasUniqueColumns(t *testing.T) {
	seen := map[string]bool{}
	for _, column := range trafficAccountingCSVHeader() {
		if seen[column] {
			t.Fatalf("duplicate CSV column %q", column)
		}
		seen[column] = true
	}
}
