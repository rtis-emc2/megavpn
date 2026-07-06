package http

import (
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
