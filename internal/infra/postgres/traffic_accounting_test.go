package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestTrafficAccountingPruneBatchQueryIsBounded(t *testing.T) {
	query := strings.ToLower(trafficAccountingPruneBatchQuery())
	for _, want := range []string{"ctid", "order by received_at asc", "limit $2"} {
		if !strings.Contains(query, want) {
			t.Fatalf("prune query must contain %q: %s", want, query)
		}
	}
	if strings.Contains(query, "delete from traffic_accounting_samples where received_at < $1") {
		t.Fatalf("prune query must not use an unbounded retention delete: %s", query)
	}
}

func TestTrafficAccountingPruneConstantsBoundIngestWork(t *testing.T) {
	if trafficAccountingPruneBatchSize <= 0 {
		t.Fatalf("trafficAccountingPruneBatchSize must be positive")
	}
	if trafficAccountingPruneBatchesPerIngest <= 0 {
		t.Fatalf("trafficAccountingPruneBatchesPerIngest must be positive")
	}
	if trafficAccountingPruneBatchSize*trafficAccountingPruneBatchesPerIngest > 100000 {
		t.Fatalf("traffic accounting prune budget is too large for an ingest path")
	}
	if got := trafficAccountingPruneBudget(); got != trafficAccountingPruneBatchSize*trafficAccountingPruneBatchesPerIngest {
		t.Fatalf("trafficAccountingPruneBudget() = %d, want configured batch budget", got)
	}
}

func TestTrafficAccountingRetentionCutoffUsesDomainPolicy(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 34, 56, 0, time.FixedZone("test", 3*60*60))
	got := trafficAccountingRetentionCutoff(now)
	want := now.UTC().AddDate(0, 0, -domain.TrafficAccountingRetentionDays)
	if !got.Equal(want) {
		t.Fatalf("cutoff = %s, want %s", got, want)
	}
}
