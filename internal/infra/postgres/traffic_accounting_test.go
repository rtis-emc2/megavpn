package postgres

import (
	"reflect"
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

func TestTrafficAccountingFilterLimitClampsSafely(t *testing.T) {
	if got := trafficAccountingFilterLimit(0, 1000, 200); got != 200 {
		t.Fatalf("zero limit = %d, want fallback", got)
	}
	if got := trafficAccountingFilterLimit(-1, 1000, 200); got != 200 {
		t.Fatalf("negative limit = %d, want fallback", got)
	}
	if got := trafficAccountingFilterLimit(1001, 1000, 200); got != 1000 {
		t.Fatalf("oversized limit = %d, want clamp to max", got)
	}
	if got := trafficAccountingFilterLimit(500, 1000, 200); got != 500 {
		t.Fatalf("valid limit = %d, want requested value", got)
	}
}

func TestTrafficAccountingFilterWhereBuildsBoundedPredicates(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.FixedZone("operator", 3*60*60))
	to := time.Date(2026, 7, 6, 23, 59, 59, 0, time.FixedZone("operator", 3*60*60))
	args, where, err := trafficAccountingFilterWhere(domain.TrafficAccountingExportFilter{
		From:            &from,
		To:              &to,
		ClientAccountID: "00000000-0000-0000-0000-000000000001",
		NodeID:          "00000000-0000-0000-0000-000000000002",
		Protocol:        " WireGuard ",
	}, cutoff)
	if err != nil {
		t.Fatalf("build filter predicates: %v", err)
	}
	wantWhere := []string{
		"t.received_at >= $1",
		"t.bucket_end >= $2",
		"t.bucket_start <= $3",
		"t.client_account_id = $4::uuid",
		"t.node_id = $5::uuid",
		"t.protocol = $6",
	}
	if !reflect.DeepEqual(where, wantWhere) {
		t.Fatalf("where = %#v, want %#v", where, wantWhere)
	}
	if len(args) != 6 {
		t.Fatalf("args len = %d, want 6", len(args))
	}
	if got := args[1].(time.Time); !got.Equal(from.UTC()) {
		t.Fatalf("from arg = %s, want %s", got, from.UTC())
	}
	if got := args[2].(time.Time); !got.Equal(to.UTC()) {
		t.Fatalf("to arg = %s, want %s", got, to.UTC())
	}
	if args[3] != "00000000-0000-0000-0000-000000000001" || args[4] != "00000000-0000-0000-0000-000000000002" || args[5] != "wireguard" {
		t.Fatalf("unexpected scalar args: %#v", args)
	}
}

func TestTrafficAccountingFilterWhereRejectsInvalidUUIDs(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := trafficAccountingFilterWhere(domain.TrafficAccountingExportFilter{ClientAccountID: "not-a-uuid"}, cutoff); err == nil {
		t.Fatal("expected invalid client_id to fail")
	}
	if _, _, err := trafficAccountingFilterWhere(domain.TrafficAccountingExportFilter{NodeID: "not-a-uuid"}, cutoff); err == nil {
		t.Fatal("expected invalid node_id to fail")
	}
}
