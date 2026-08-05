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

func TestTrafficAccountingCollectorStatusQueryUsesAppliedActiveManagedInstances(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	query, args := trafficAccountingCollectorStatusQuery([]any{cutoff}, []string{"t.received_at >= $1"}, domain.TrafficAccountingExportFilter{})
	for _, want := range []string{
		"i.enabled=true",
		"i.status in ('active','degraded')",
		"sd.code in ('xray-core','wireguard','openvpn')",
		"r.id=coalesce(i.last_applied_revision_id, i.current_revision_id)",
		"count(distinct i.id) as expected_instance_count",
		"full join expected",
		"limit $2",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("collector query missing %q:\n%s", want, query)
		}
	}
	for _, banned := range []string{
		"i.status not in",
		"r.id=i.current_revision_id",
	} {
		if strings.Contains(query, banned) {
			t.Fatalf("collector query contains stale predicate %q:\n%s", banned, query)
		}
	}
	if len(args) != 2 || args[0] != cutoff || args[1] != trafficAccountingMaxCollectorRows {
		t.Fatalf("collector args = %#v, want cutoff and limit", args)
	}
}

func TestTrafficAccountingCollectorStatusQueryKeepsExpectedFiltersBounded(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	filter := domain.TrafficAccountingExportFilter{
		ClientAccountID: "00000000-0000-0000-0000-000000000001",
		NodeID:          "00000000-0000-0000-0000-000000000002",
		Protocol:        " WireGuard ",
	}
	baseArgs, where, err := trafficAccountingFilterWhere(filter, cutoff)
	if err != nil {
		t.Fatalf("build base filter: %v", err)
	}
	query, args := trafficAccountingCollectorStatusQuery(baseArgs, where, filter)
	for _, want := range []string{
		"t.client_account_id = $2::uuid",
		"t.node_id = $3::uuid",
		"t.protocol = $4",
		"false",
		"i.node_id = $5::uuid",
		"end = $6",
		"limit $7",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("collector filtered query missing %q:\n%s", want, query)
		}
	}
	if len(args) != 7 {
		t.Fatalf("collector filtered args len = %d, want 7: %#v", len(args), args)
	}
	if args[4] != filter.NodeID || args[5] != "wireguard" || args[6] != trafficAccountingMaxCollectorRows {
		t.Fatalf("collector expected filter args = %#v", args)
	}
}

func TestTrafficAccountingClientUsageQueryUsesRetentionScopedFilters(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	filter := domain.TrafficAccountingExportFilter{
		ClientAccountID: "00000000-0000-0000-0000-000000000001",
		NodeID:          "00000000-0000-0000-0000-000000000002",
		Protocol:        "OpenVPN",
	}
	baseArgs, where, err := trafficAccountingFilterWhere(filter, cutoff)
	if err != nil {
		t.Fatalf("build base filter: %v", err)
	}
	query, args := trafficAccountingClientUsageQuery(baseArgs, where)
	for _, want := range []string{
		"t.received_at >= $1",
		"t.client_account_id = $2::uuid",
		"t.node_id = $3::uuid",
		"t.protocol = $4",
		"t.client_account_id is not null",
		"group by t.client_account_id, c.username",
		"limit $5",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("client usage query missing %q:\n%s", want, query)
		}
	}
	if len(args) != 5 {
		t.Fatalf("client usage args len = %d, want 5: %#v", len(args), args)
	}
	if args[0] != cutoff || args[1] != filter.ClientAccountID || args[2] != filter.NodeID || args[3] != "openvpn" || args[4] != trafficAccountingMaxClientRows {
		t.Fatalf("client usage args = %#v", args)
	}
}

func TestTrafficAccountingCollectorStatusClassifiesFreshness(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	activeAt := now.Add(-trafficAccountingCollectorActiveWindow)
	status, age := trafficAccountingCollectorStatus(now, &activeAt, 0)
	if status != "active" || age != int64(trafficAccountingCollectorActiveWindow.Seconds()) {
		t.Fatalf("active boundary = %s/%d", status, age)
	}
	warnAt := now.Add(-trafficAccountingCollectorWarnWindow)
	status, age = trafficAccountingCollectorStatus(now, &warnAt, 0)
	if status != "degraded" || age != int64(trafficAccountingCollectorWarnWindow.Seconds()) {
		t.Fatalf("degraded boundary = %s/%d", status, age)
	}
	inactiveAt := now.Add(-(trafficAccountingCollectorWarnWindow + time.Second))
	status, age = trafficAccountingCollectorStatus(now, &inactiveAt, 0)
	if status != "inactive" || age != int64((trafficAccountingCollectorWarnWindow+time.Second).Seconds()) {
		t.Fatalf("inactive boundary = %s/%d", status, age)
	}
	futureAt := now.Add(time.Minute)
	status, age = trafficAccountingCollectorStatus(now, &futureAt, 0)
	if status != "active" || age != 0 {
		t.Fatalf("future clock skew = %s/%d, want active/0", status, age)
	}
	status, age = trafficAccountingCollectorStatus(now, nil, 1)
	if status != "missing" || age != 0 {
		t.Fatalf("missing expected stream = %s/%d, want missing/0", status, age)
	}
	status, age = trafficAccountingCollectorStatus(now, &futureAt, 1)
	if status != "degraded" || age != 0 {
		t.Fatalf("partial expected stream = %s/%d, want degraded/0", status, age)
	}
}
