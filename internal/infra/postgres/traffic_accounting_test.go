package postgres

import (
	"strings"
	"testing"
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
}
