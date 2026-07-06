package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const (
	trafficAccountingMaxBatch              = 1000
	trafficAccountingMaxRows               = 1000
	trafficAccountingMaxExportRows         = 50000
	trafficAccountingMaxCollectorRows      = 100
	trafficAccountingCollectorActiveWindow = 5 * time.Minute
	trafficAccountingCollectorWarnWindow   = 30 * time.Minute
	trafficAccountingMaxBucketSpan         = 24 * time.Hour
	trafficAccountingPruneBatchSize        = 5000
	trafficAccountingPruneBatchesPerIngest = 10
)

var trafficAccountingUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type trafficAccountingSampleInput struct {
	ID              string
	NodeID          string
	InstanceID      string
	ServiceAccessID string
	ClientAccountID string
	SampleKey       string
	Source          string
	Protocol        string
	Direction       string
	BucketStart     time.Time
	BucketEnd       time.Time
	RxBytes         int64
	TxBytes         int64
	RxPackets       int64
	TxPackets       int64
	FlowCount       int64
	Metadata        map[string]any
	ObservedAt      time.Time
}

func (s *Store) TrafficAccountingOverview(ctx context.Context, filter domain.TrafficAccountingExportFilter) (domain.TrafficAccountingOverview, error) {
	limit := trafficAccountingFilterLimit(filter.Limit, trafficAccountingMaxRows, 200)
	now := time.Now().UTC()
	cutoff := trafficAccountingRetentionCutoff(now)
	args, where, err := trafficAccountingFilterWhere(filter, cutoff)
	if err != nil {
		return domain.TrafficAccountingOverview{}, err
	}
	var out domain.TrafficAccountingOverview
	out.Summary.RetentionDays = domain.TrafficAccountingRetentionDays
	out.Summary.RetentionCutoff = &cutoff
	out.Summary.PruneBatchSize = trafficAccountingPruneBatchSize
	out.Summary.PruneBatchesPerIngest = trafficAccountingPruneBatchesPerIngest
	out.Summary.MaxPrunePerIngest = trafficAccountingPruneBudget()
	summaryQuery := `select
			count(*),
			count(distinct client_account_id) filter(where client_account_id is not null),
			count(distinct node_id),
			coalesce(sum(rx_bytes), 0),
			coalesce(sum(tx_bytes), 0),
			coalesce(sum(flow_count), 0),
			min(bucket_start),
			max(bucket_end)
		from traffic_accounting_samples t
		where ` + strings.Join(where, " and ")
	err = s.db.QueryRow(ctx, summaryQuery, args...).Scan(
		&out.Summary.SampleCount,
		&out.Summary.ClientCount,
		&out.Summary.NodeCount,
		&out.Summary.RxBytes,
		&out.Summary.TxBytes,
		&out.Summary.FlowCount,
		&out.Summary.OldestBucketStart,
		&out.Summary.NewestBucketEnd,
	)
	if err != nil {
		return out, err
	}
	if err := s.db.QueryRow(ctx, `select count(*) from traffic_accounting_samples where received_at < $1`, cutoff).Scan(&out.Summary.ExpiredSampleCount); err != nil {
		return out, err
	}
	out.Collectors, err = s.trafficAccountingCollectorStatuses(ctx, args, where, filter, now)
	if err != nil {
		return out, err
	}

	args = append(args, limit)
	rowsQuery := `select
			t.id::text,
			t.node_id::text,
			coalesce(n.name, ''),
			coalesce(t.instance_id::text, ''),
			coalesce(i.name, ''),
			coalesce(t.service_access_id::text, ''),
			coalesce(t.client_account_id::text, ''),
			coalesce(c.username, ''),
			t.source,
			t.protocol,
			t.direction,
			t.bucket_start,
			t.bucket_end,
			t.rx_bytes,
			t.tx_bytes,
			t.rx_packets,
			t.tx_packets,
			t.flow_count,
			t.metadata_json,
			t.observed_at,
			t.received_at
		from traffic_accounting_samples t
		left join nodes n on n.id=t.node_id
		left join instances i on i.id=t.instance_id
		left join client_accounts c on c.id=t.client_account_id
		where ` + strings.Join(where, " and ") + `
		order by t.bucket_end desc, t.received_at desc
		limit $` + strconv.Itoa(len(args))
	rows, err := s.db.Query(ctx, rowsQuery, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanTrafficAccountingSample(rows)
		if err != nil {
			return out, err
		}
		out.Samples = append(out.Samples, item)
	}
	if out.Samples == nil {
		out.Samples = []domain.TrafficAccountingSample{}
	}
	if out.Collectors == nil {
		out.Collectors = []domain.TrafficAccountingCollectorStatus{}
	}
	return out, rows.Err()
}

func (s *Store) trafficAccountingCollectorStatuses(ctx context.Context, args []any, where []string, filter domain.TrafficAccountingExportFilter, now time.Time) ([]domain.TrafficAccountingCollectorStatus, error) {
	queryArgs := append([]any{}, args...)
	expectedWhere := []string{
		"i.enabled=true",
		"i.status not in ('deleted','disabled')",
		"sd.code in ('xray-core','wireguard','openvpn')",
		`(
			lower(coalesce(r.spec_json->>'traffic_accounting_enabled','')) in ('true','1','yes','on','enabled')
			or (sd.code='xray-core' and lower(coalesce(r.spec_json->>'xray_stats_enabled','')) in ('true','1','yes','on','enabled'))
			or (sd.code='xray-core' and lower(coalesce(r.spec_json->>'stats_enabled','')) in ('true','1','yes','on','enabled'))
		)`,
	}
	if strings.TrimSpace(filter.ClientAccountID) != "" {
		expectedWhere = append(expectedWhere, "false")
	}
	if nodeID := strings.TrimSpace(filter.NodeID); nodeID != "" {
		queryArgs = append(queryArgs, nodeID)
		expectedWhere = append(expectedWhere, fmt.Sprintf("i.node_id = $%d::uuid", len(queryArgs)))
	}
	if protocol := normalizeTrafficAccountingToken(filter.Protocol, ""); protocol != "" {
		queryArgs = append(queryArgs, protocol)
		expectedWhere = append(expectedWhere, fmt.Sprintf(`case
			when sd.code='xray-core' then 'vless'
			when sd.code='wireguard' then 'wireguard'
			when sd.code='openvpn' then 'openvpn'
			else sd.code
		end = $%d`, len(queryArgs)))
	}
	queryArgs = append(queryArgs, trafficAccountingMaxCollectorRows)
	query := `with observed as (
		select
			t.node_id::text as node_id,
			coalesce(n.name, '') as node_name,
			t.source,
			t.protocol,
			count(*) as sample_count,
			count(distinct t.client_account_id) filter(where t.client_account_id is not null) as client_count,
			count(distinct t.instance_id) filter(where t.instance_id is not null) as observed_instance_count,
			coalesce(sum(t.rx_bytes), 0) as rx_bytes,
			coalesce(sum(t.tx_bytes), 0) as tx_bytes,
			coalesce(sum(t.flow_count), 0) as flow_count,
			max(t.bucket_end) as last_bucket_end,
			max(t.received_at) as last_received_at
		from traffic_accounting_samples t
		left join nodes n on n.id=t.node_id
		where ` + strings.Join(where, " and ") + `
		group by t.node_id, n.name, t.source, t.protocol
	), expected as (
		select
			i.node_id::text as node_id,
			coalesce(n.name, '') as node_name,
			'agent'::text as source,
			case
				when sd.code='xray-core' then 'vless'
				when sd.code='wireguard' then 'wireguard'
				when sd.code='openvpn' then 'openvpn'
				else sd.code
			end as protocol,
			count(distinct i.id) as expected_instance_count
		from instances i
		join service_definitions sd on sd.id=i.service_definition_id
		join instance_revisions r on r.id=i.current_revision_id
		left join nodes n on n.id=i.node_id
		where ` + strings.Join(expectedWhere, " and ") + `
		group by i.node_id, n.name, case
			when sd.code='xray-core' then 'vless'
			when sd.code='wireguard' then 'wireguard'
			when sd.code='openvpn' then 'openvpn'
			else sd.code
		end
	)
	select
		coalesce(o.node_id, e.node_id),
		coalesce(nullif(o.node_name,''), nullif(e.node_name,''), ''),
		coalesce(o.source, e.source),
		coalesce(o.protocol, e.protocol),
		coalesce(o.sample_count, 0),
		coalesce(o.client_count, 0),
		coalesce(o.rx_bytes, 0),
		coalesce(o.tx_bytes, 0),
		coalesce(o.flow_count, 0),
		coalesce(e.expected_instance_count, 0),
		coalesce(o.observed_instance_count, 0),
		greatest(coalesce(e.expected_instance_count, 0) - coalesce(o.observed_instance_count, 0), 0),
		o.last_bucket_end,
		o.last_received_at
	from observed o
	full join expected e on e.node_id=o.node_id and e.source=o.source and e.protocol=o.protocol
	order by coalesce(o.last_received_at, to_timestamp(0)) desc, coalesce(o.protocol, e.protocol) asc, coalesce(o.source, e.source) asc
		limit $` + strconv.Itoa(len(queryArgs))
	rows, err := s.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TrafficAccountingCollectorStatus, 0)
	for rows.Next() {
		item, err := scanTrafficAccountingCollectorStatus(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if out == nil {
		out = []domain.TrafficAccountingCollectorStatus{}
	}
	return out, rows.Err()
}

func (s *Store) TrafficAccountingSamples(ctx context.Context, filter domain.TrafficAccountingExportFilter) ([]domain.TrafficAccountingSample, error) {
	limit := trafficAccountingFilterLimit(filter.Limit, trafficAccountingMaxExportRows, trafficAccountingMaxExportRows)
	args, where, err := trafficAccountingFilterWhere(filter, trafficAccountingRetentionCutoff(time.Now().UTC()))
	if err != nil {
		return nil, err
	}
	args = append(args, limit)
	query := `select
			t.id::text,
			t.node_id::text,
			coalesce(n.name, ''),
			coalesce(t.instance_id::text, ''),
			coalesce(i.name, ''),
			coalesce(t.service_access_id::text, ''),
			coalesce(t.client_account_id::text, ''),
			coalesce(c.username, ''),
			t.source,
			t.protocol,
			t.direction,
			t.bucket_start,
			t.bucket_end,
			t.rx_bytes,
			t.tx_bytes,
			t.rx_packets,
			t.tx_packets,
			t.flow_count,
			t.metadata_json,
			t.observed_at,
			t.received_at
		from traffic_accounting_samples t
		left join nodes n on n.id=t.node_id
		left join instances i on i.id=t.instance_id
		left join client_accounts c on c.id=t.client_account_id
		where ` + strings.Join(where, " and ") + `
		order by t.bucket_end desc, t.received_at desc
		limit $` + strconv.Itoa(len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TrafficAccountingSample, 0)
	for rows.Next() {
		item, err := scanTrafficAccountingSample(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if out == nil {
		out = []domain.TrafficAccountingSample{}
	}
	return out, rows.Err()
}

func trafficAccountingFilterLimit(limit, max, fallback int) int {
	if limit <= 0 {
		return fallback
	}
	if limit > max {
		return max
	}
	return limit
}

func trafficAccountingFilterWhere(filter domain.TrafficAccountingExportFilter, cutoff time.Time) ([]any, []string, error) {
	args := []any{cutoff}
	where := []string{"t.received_at >= $1"}
	if filter.From != nil && !filter.From.IsZero() {
		args = append(args, filter.From.UTC())
		where = append(where, fmt.Sprintf("t.bucket_end >= $%d", len(args)))
	}
	if filter.To != nil && !filter.To.IsZero() {
		args = append(args, filter.To.UTC())
		where = append(where, fmt.Sprintf("t.bucket_start <= $%d", len(args)))
	}
	if clientID := strings.TrimSpace(filter.ClientAccountID); clientID != "" {
		if !trafficAccountingUUIDPattern.MatchString(clientID) {
			return nil, nil, fmt.Errorf("client_id must be a UUID")
		}
		args = append(args, clientID)
		where = append(where, fmt.Sprintf("t.client_account_id = $%d::uuid", len(args)))
	}
	if nodeID := strings.TrimSpace(filter.NodeID); nodeID != "" {
		if !trafficAccountingUUIDPattern.MatchString(nodeID) {
			return nil, nil, fmt.Errorf("node_id must be a UUID")
		}
		args = append(args, nodeID)
		where = append(where, fmt.Sprintf("t.node_id = $%d::uuid", len(args)))
	}
	if protocol := normalizeTrafficAccountingToken(filter.Protocol, ""); protocol != "" {
		args = append(args, protocol)
		where = append(where, fmt.Sprintf("t.protocol = $%d", len(args)))
	}
	return args, where, nil
}

func (s *Store) SubmitAgentTrafficAccountingSamples(ctx context.Context, nodeID string, samples []domain.AgentTrafficAccountingSample) (domain.TrafficAccountingIngestResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	result := domain.TrafficAccountingIngestResult{
		Status:        "accepted",
		RetentionDays: domain.TrafficAccountingRetentionDays,
	}
	if nodeID == "" {
		return result, errors.New("node_id is required")
	}
	if !trafficAccountingUUIDPattern.MatchString(nodeID) {
		return result, fmt.Errorf("node_id must be a UUID")
	}
	if len(samples) > trafficAccountingMaxBatch {
		samples = samples[:trafficAccountingMaxBatch]
	}
	for idx, sample := range samples {
		input, err := s.normalizeTrafficAccountingSample(ctx, nodeID, sample)
		if err != nil {
			result.Rejected++
			result.Errors = append(result.Errors, fmt.Sprintf("sample %d: %v", idx, err))
			continue
		}
		if err := s.upsertTrafficAccountingSample(ctx, input); err != nil {
			return result, err
		}
		result.Accepted++
		result.Inserted++
	}
	if _, err := s.db.Exec(ctx, `update node_agents set last_seen_at=now(), status='active' where node_id=$1`, nodeID); err != nil {
		return result, err
	}
	pruned, err := s.pruneTrafficAccountingSamples(ctx)
	if err != nil {
		return result, err
	}
	result.Pruned = pruned
	if result.Rejected > 0 && result.Accepted == 0 {
		result.Status = "rejected"
	} else if result.Rejected > 0 {
		result.Status = "partial"
	}
	return result, nil
}

func scanTrafficAccountingSample(row interface{ Scan(dest ...any) error }) (domain.TrafficAccountingSample, error) {
	var item domain.TrafficAccountingSample
	var metadataRaw []byte
	if err := row.Scan(
		&item.ID,
		&item.NodeID,
		&item.NodeName,
		&item.InstanceID,
		&item.InstanceName,
		&item.ServiceAccessID,
		&item.ClientAccountID,
		&item.ClientUsername,
		&item.Source,
		&item.Protocol,
		&item.Direction,
		&item.BucketStart,
		&item.BucketEnd,
		&item.RxBytes,
		&item.TxBytes,
		&item.RxPackets,
		&item.TxPackets,
		&item.FlowCount,
		&metadataRaw,
		&item.ObservedAt,
		&item.ReceivedAt,
	); err != nil {
		return domain.TrafficAccountingSample{}, err
	}
	_ = json.Unmarshal(metadataRaw, &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
}

func scanTrafficAccountingCollectorStatus(row interface{ Scan(dest ...any) error }, now time.Time) (domain.TrafficAccountingCollectorStatus, error) {
	var item domain.TrafficAccountingCollectorStatus
	var lastBucketEnd sql.NullTime
	var lastReceivedAt sql.NullTime
	if err := row.Scan(
		&item.NodeID,
		&item.NodeName,
		&item.Source,
		&item.Protocol,
		&item.SampleCount,
		&item.ClientCount,
		&item.RxBytes,
		&item.TxBytes,
		&item.FlowCount,
		&item.ExpectedInstanceCount,
		&item.ObservedInstanceCount,
		&item.MissingInstanceCount,
		&lastBucketEnd,
		&lastReceivedAt,
	); err != nil {
		return domain.TrafficAccountingCollectorStatus{}, err
	}
	if lastBucketEnd.Valid {
		item.LastBucketEnd = &lastBucketEnd.Time
	}
	if lastReceivedAt.Valid {
		item.LastReceivedAt = &lastReceivedAt.Time
	}
	item.Status, item.LastReceivedAgeSeconds = trafficAccountingCollectorStatus(now, item.LastReceivedAt, item.MissingInstanceCount)
	return item, nil
}

func trafficAccountingCollectorStatus(now time.Time, lastReceivedAt *time.Time, missingInstances int64) (string, int64) {
	if lastReceivedAt == nil || lastReceivedAt.IsZero() {
		if missingInstances > 0 {
			return "missing", 0
		}
		return "inactive", 0
	}
	age := now.UTC().Sub(lastReceivedAt.UTC())
	if age < 0 {
		age = 0
	}
	switch {
	case age <= trafficAccountingCollectorActiveWindow:
		if missingInstances > 0 {
			return "degraded", int64(age.Seconds())
		}
		return "active", int64(age.Seconds())
	case age <= trafficAccountingCollectorWarnWindow:
		return "degraded", int64(age.Seconds())
	default:
		return "inactive", int64(age.Seconds())
	}
}

func (s *Store) normalizeTrafficAccountingSample(ctx context.Context, nodeID string, sample domain.AgentTrafficAccountingSample) (trafficAccountingSampleInput, error) {
	now := time.Now().UTC()
	input := trafficAccountingSampleInput{
		ID:              id.New(),
		NodeID:          nodeID,
		InstanceID:      strings.TrimSpace(sample.InstanceID),
		ServiceAccessID: strings.TrimSpace(sample.ServiceAccessID),
		ClientAccountID: strings.TrimSpace(sample.ClientAccountID),
		SampleKey:       strings.TrimSpace(sample.SampleKey),
		Source:          normalizeTrafficAccountingSource(sample.Source),
		Protocol:        normalizeTrafficAccountingToken(sample.Protocol, "unknown"),
		Direction:       normalizeTrafficAccountingDirection(sample.Direction),
		RxBytes:         sample.RxBytes,
		TxBytes:         sample.TxBytes,
		RxPackets:       sample.RxPackets,
		TxPackets:       sample.TxPackets,
		FlowCount:       sample.FlowCount,
		Metadata:        sample.Metadata,
		ObservedAt:      now,
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	if sample.ObservedAt != nil && !sample.ObservedAt.IsZero() {
		input.ObservedAt = sample.ObservedAt.UTC()
	}
	if sample.BucketStart != nil && !sample.BucketStart.IsZero() {
		input.BucketStart = sample.BucketStart.UTC()
	}
	if sample.BucketEnd != nil && !sample.BucketEnd.IsZero() {
		input.BucketEnd = sample.BucketEnd.UTC()
	}
	if input.BucketStart.IsZero() && input.BucketEnd.IsZero() {
		input.BucketEnd = now.Truncate(time.Minute)
		input.BucketStart = input.BucketEnd.Add(-time.Minute)
	} else if input.BucketStart.IsZero() {
		input.BucketStart = input.BucketEnd.Add(-time.Minute)
	} else if input.BucketEnd.IsZero() {
		input.BucketEnd = input.BucketStart.Add(time.Minute)
	}
	if input.BucketEnd.Before(input.BucketStart) || input.BucketEnd.Equal(input.BucketStart) {
		return input, errors.New("bucket_end must be after bucket_start")
	}
	if input.BucketEnd.Sub(input.BucketStart) > trafficAccountingMaxBucketSpan {
		return input, fmt.Errorf("bucket span must not exceed %s", trafficAccountingMaxBucketSpan)
	}
	if input.RxBytes < 0 || input.TxBytes < 0 || input.RxPackets < 0 || input.TxPackets < 0 || input.FlowCount < 0 {
		return input, errors.New("traffic counters must be non-negative")
	}
	if input.RxBytes == 0 && input.TxBytes == 0 && input.RxPackets == 0 && input.TxPackets == 0 && input.FlowCount == 0 {
		return input, errors.New("at least one traffic counter must be greater than zero")
	}
	if err := validateOptionalTrafficAccountingUUID("instance_id", input.InstanceID); err != nil {
		return input, err
	}
	if err := validateOptionalTrafficAccountingUUID("service_access_id", input.ServiceAccessID); err != nil {
		return input, err
	}
	if err := validateOptionalTrafficAccountingUUID("client_account_id", input.ClientAccountID); err != nil {
		return input, err
	}
	if input.ClientAccountID == "" {
		username := trafficAccountingMetadataString(input.Metadata, "client_user", "client_username", "xray_user", "user", "email")
		if username != "" {
			clientID, err := s.trafficAccountingClientIDByUsername(ctx, username)
			if err != nil {
				return input, err
			}
			input.ClientAccountID = clientID
		}
	}
	if input.InstanceID != "" {
		ok, err := s.trafficAccountingInstanceBelongsToNode(ctx, input.InstanceID, nodeID)
		if err != nil {
			return input, err
		}
		if !ok {
			return input, fmt.Errorf("instance_id %s does not belong to reporting node", input.InstanceID)
		}
	}
	if input.ServiceAccessID == "" && input.InstanceID != "" {
		accessID, clientID, err := s.trafficAccountingServiceAccessByMetadata(ctx, input.InstanceID, input.Metadata)
		if err != nil {
			return input, err
		}
		if accessID != "" {
			input.ServiceAccessID = accessID
			if input.ClientAccountID == "" {
				input.ClientAccountID = clientID
			} else if clientID != "" && input.ClientAccountID != clientID {
				return input, fmt.Errorf("traffic metadata matches service access for a different client")
			}
		}
	}
	if input.ServiceAccessID != "" {
		accessInstanceID, accessClientID, err := s.trafficAccountingServiceAccessRefs(ctx, input.ServiceAccessID)
		if err != nil {
			return input, err
		}
		if input.InstanceID == "" {
			input.InstanceID = accessInstanceID
		} else if input.InstanceID != accessInstanceID {
			return input, fmt.Errorf("service_access_id %s belongs to a different instance", input.ServiceAccessID)
		}
		if input.ClientAccountID == "" {
			input.ClientAccountID = accessClientID
		} else if input.ClientAccountID != accessClientID {
			return input, fmt.Errorf("service_access_id %s belongs to a different client", input.ServiceAccessID)
		}
		if ok, err := s.trafficAccountingInstanceBelongsToNode(ctx, input.InstanceID, nodeID); err != nil {
			return input, err
		} else if !ok {
			return input, fmt.Errorf("service_access_id %s belongs to a different node", input.ServiceAccessID)
		}
	}
	if input.ServiceAccessID == "" && input.InstanceID != "" && input.ClientAccountID != "" {
		accessID, err := s.trafficAccountingServiceAccessID(ctx, input.ClientAccountID, input.InstanceID)
		if err != nil {
			return input, err
		}
		input.ServiceAccessID = accessID
	}
	input.SampleKey = trafficAccountingSampleKey(nodeID, input)
	return input, nil
}

func (s *Store) upsertTrafficAccountingSample(ctx context.Context, input trafficAccountingSampleInput) error {
	_, err := s.db.Exec(ctx, `insert into traffic_accounting_samples(
			id,node_id,instance_id,service_access_id,client_account_id,sample_key,source,protocol,direction,
			bucket_start,bucket_end,rx_bytes,tx_bytes,rx_packets,tx_packets,flow_count,metadata_json,observed_at,received_at
		) values(
			$1,$2,nullif($3,'')::uuid,nullif($4,'')::uuid,nullif($5,'')::uuid,$6,$7,$8,$9,
			$10,$11,$12,$13,$14,$15,$16,$17,$18,now()
		)
		on conflict(node_id, sample_key) where sample_key <> '' do update set
			instance_id=excluded.instance_id,
			service_access_id=excluded.service_access_id,
			client_account_id=excluded.client_account_id,
			source=excluded.source,
			protocol=excluded.protocol,
			direction=excluded.direction,
			bucket_start=excluded.bucket_start,
			bucket_end=excluded.bucket_end,
			rx_bytes=excluded.rx_bytes,
			tx_bytes=excluded.tx_bytes,
			rx_packets=excluded.rx_packets,
			tx_packets=excluded.tx_packets,
			flow_count=excluded.flow_count,
			metadata_json=excluded.metadata_json,
			observed_at=excluded.observed_at,
			received_at=now()`,
		input.ID,
		input.NodeID,
		input.InstanceID,
		input.ServiceAccessID,
		input.ClientAccountID,
		input.SampleKey,
		input.Source,
		input.Protocol,
		input.Direction,
		input.BucketStart,
		input.BucketEnd,
		input.RxBytes,
		input.TxBytes,
		input.RxPackets,
		input.TxPackets,
		input.FlowCount,
		mustJSON(input.Metadata),
		input.ObservedAt,
	)
	return err
}

func (s *Store) pruneTrafficAccountingSamples(ctx context.Context) (int64, error) {
	cutoff := trafficAccountingRetentionCutoff(time.Now().UTC())
	var total int64
	for batch := 0; batch < trafficAccountingPruneBatchesPerIngest; batch++ {
		affected, err := s.pruneTrafficAccountingSamplesBatch(ctx, cutoff, trafficAccountingPruneBatchSize)
		if err != nil {
			return total, err
		}
		total += affected
		if affected < int64(trafficAccountingPruneBatchSize) {
			break
		}
	}
	return total, nil
}

func (s *Store) pruneTrafficAccountingSamplesBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	tag, err := s.db.Exec(ctx, trafficAccountingPruneBatchQuery(), cutoff, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func trafficAccountingPruneBatchQuery() string {
	return `delete from traffic_accounting_samples
		where ctid in (
			select ctid
			from traffic_accounting_samples
			where received_at < $1
			order by received_at asc
			limit $2
		)`
}

func trafficAccountingRetentionCutoff(now time.Time) time.Time {
	return now.UTC().AddDate(0, 0, -domain.TrafficAccountingRetentionDays)
}

func trafficAccountingPruneBudget() int {
	return trafficAccountingPruneBatchSize * trafficAccountingPruneBatchesPerIngest
}

func (s *Store) trafficAccountingInstanceBelongsToNode(ctx context.Context, instanceID, nodeID string) (bool, error) {
	var found bool
	err := s.db.QueryRow(ctx, `select exists(select 1 from instances where id=$1 and node_id=$2 and status <> 'deleted')`, instanceID, nodeID).Scan(&found)
	return found, err
}

func (s *Store) trafficAccountingServiceAccessRefs(ctx context.Context, accessID string) (string, string, error) {
	var instanceID, clientID string
	err := s.db.QueryRow(ctx, `select instance_id::text, client_account_id::text from service_accesses where id=$1 and status <> 'revoked'`, accessID).Scan(&instanceID, &clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("service_access_id %s is not active", accessID)
	}
	return instanceID, clientID, err
}

func (s *Store) trafficAccountingClientIDByUsername(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", nil
	}
	var clientID string
	err := s.db.QueryRow(ctx, `select id::text from client_accounts where username=$1 and status <> 'deleted' order by created_at desc limit 1`, username).Scan(&clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return clientID, err
}

func (s *Store) trafficAccountingServiceAccessID(ctx context.Context, clientID, instanceID string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	instanceID = strings.TrimSpace(instanceID)
	if clientID == "" || instanceID == "" {
		return "", nil
	}
	var accessID string
	err := s.db.QueryRow(ctx, `select id::text
		from service_accesses
		where client_account_id=$1
		  and instance_id=$2
		  and status in ('active','pending')
		order by case status when 'active' then 0 when 'pending' then 1 else 2 end, updated_at desc
		limit 1`, clientID, instanceID).Scan(&accessID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return accessID, err
}

func (s *Store) trafficAccountingServiceAccessByMetadata(ctx context.Context, instanceID string, metadata map[string]any) (string, string, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" || metadata == nil {
		return "", "", nil
	}
	matchers := []struct {
		Key   string
		Value string
	}{
		{Key: "wireguard_client_public_key", Value: trafficAccountingMetadataString(metadata, "wireguard_client_public_key")},
		{Key: "wireguard_client_address", Value: trafficAccountingFirstListValue(trafficAccountingMetadataString(metadata, "wireguard_client_address", "wireguard_allowed_ip"))},
		{Key: "openvpn_client_common_name", Value: trafficAccountingMetadataString(metadata, "openvpn_client_common_name", "openvpn_common_name")},
	}
	for _, matcher := range matchers {
		value := strings.TrimSpace(matcher.Value)
		if value == "" {
			continue
		}
		var accessID, clientID string
		err := s.db.QueryRow(ctx, `select id::text, client_account_id::text
			from service_accesses
			where instance_id=$1
			  and status in ('active','pending')
			  and metadata_json->>$2=$3
			order by case status when 'active' then 0 when 'pending' then 1 else 2 end, updated_at desc
			limit 1`, instanceID, matcher.Key, value).Scan(&accessID, &clientID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		return accessID, clientID, err
	}
	return "", "", nil
}

func trafficAccountingMetadataString(metadata map[string]any, keys ...string) string {
	if metadata == nil {
		return ""
	}
	for _, key := range keys {
		if text := strings.TrimSpace(stringify(metadata[key])); text != "" {
			return text
		}
	}
	return ""
}

func trafficAccountingFirstListValue(value string) string {
	for _, part := range strings.Split(value, ",") {
		if text := strings.TrimSpace(part); text != "" {
			return text
		}
	}
	return strings.TrimSpace(value)
}

func validateOptionalTrafficAccountingUUID(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !trafficAccountingUUIDPattern.MatchString(value) {
		return fmt.Errorf("%s must be a UUID", name)
	}
	return nil
}

func normalizeTrafficAccountingSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "agent":
		return "agent"
	case "import", "manual", "system":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "agent"
	}
}

func normalizeTrafficAccountingDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ingress", "egress", "bidirectional":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func normalizeTrafficAccountingToken(value, fallback string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		text = fallback
	}
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		}
		if b.Len() >= 48 {
			break
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}

func trafficAccountingSampleKey(nodeID string, input trafficAccountingSampleInput) string {
	if explicit := strings.TrimSpace(input.SampleKey); explicit != "" {
		sum := sha256.Sum256([]byte("explicit:" + nodeID + ":" + explicit))
		return hex.EncodeToString(sum[:])
	}
	parts := []string{
		nodeID,
		input.InstanceID,
		input.ServiceAccessID,
		input.ClientAccountID,
		input.Source,
		input.Protocol,
		input.Direction,
		input.BucketStart.UTC().Format(time.RFC3339Nano),
		input.BucketEnd.UTC().Format(time.RFC3339Nano),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}
