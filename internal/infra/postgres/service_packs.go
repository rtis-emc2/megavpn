package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) EnsureDefaultServicePacks(ctx context.Context, packs []domain.ServicePackDefinition) error {
	if len(packs) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := repairServicePackTemplateKeys(ctx, tx); err != nil {
		return err
	}
	for idx, pack := range packs {
		normalized, err := normalizeServicePackTemplate(pack, idx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into service_pack_templates(
			key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
			platform_notes_json,recommendations_json,components_json,status,source,version,display_order,created_at,updated_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),now())
		on conflict(key) do update set
			label=excluded.label,
			description=excluded.description,
			base_name_template=excluded.base_name_template,
			endpoint_hint=excluded.endpoint_hint,
			requires_endpoint_host=excluded.requires_endpoint_host,
			platform_notes_json=excluded.platform_notes_json,
			recommendations_json=excluded.recommendations_json,
			components_json=excluded.components_json,
			status=case
				when service_pack_templates.status in ('disabled','deleted') then service_pack_templates.status
				else excluded.status
			end,
			version=excluded.version,
			display_order=excluded.display_order,
			updated_at=now()
		where service_pack_templates.source='default'`,
			normalized.Key,
			normalized.Label,
			normalized.Description,
			normalized.BaseNameTemplate,
			normalized.EndpointHint,
			normalized.RequiresEndpointHost,
			mustJSON(normalized.PlatformNotes),
			mustJSON(normalized.Recommendations),
			mustJSON(normalized.Components),
			normalized.Status,
			normalized.Source,
			normalized.Version,
			normalized.DisplayOrder,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListServicePacks(ctx context.Context) ([]domain.ServicePackDefinition, error) {
	rows, err := s.db.Query(ctx, `select key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
		platform_notes_json,recommendations_json,components_json,status,source,version,display_order
	from (
		select distinct on (key)
			key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
			platform_notes_json,recommendations_json,components_json,status,source,version,display_order,updated_at
		from service_pack_templates
		where status='active'
		order by key,
			case when source='default' then 1 else 0 end,
			version desc,
			updated_at desc,
			display_order asc,
			label asc
	) deduplicated
	order by display_order asc,label asc,key asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ServicePackDefinition
	for rows.Next() {
		pack, err := scanServicePackRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pack)
	}
	return out, rows.Err()
}

func (s *Store) ListServicePackCatalog(ctx context.Context) ([]domain.ServicePackDefinition, error) {
	rows, err := s.db.Query(ctx, `select key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
		platform_notes_json,recommendations_json,components_json,status,source,version,display_order
	from (
		select distinct on (key)
			key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
			platform_notes_json,recommendations_json,components_json,status,source,version,display_order,updated_at
		from service_pack_templates
		order by key,
			case status when 'active' then 0 when 'disabled' then 1 else 2 end,
			case when source='default' then 1 else 0 end,
			version desc,
			updated_at desc,
			display_order asc,
			label asc
	) deduplicated
	order by case status when 'active' then 0 when 'disabled' then 1 else 2 end, display_order asc,label asc,key asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ServicePackDefinition
	for rows.Next() {
		pack, err := scanServicePackRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pack)
	}
	return out, rows.Err()
}

func (s *Store) GetServicePack(ctx context.Context, key string) (domain.ServicePackDefinition, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
	}
	row := s.db.QueryRow(ctx, `select key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
		platform_notes_json,recommendations_json,components_json,status,source,version,display_order
		from service_pack_templates
		where key=$1 and status='active'`, key)
	pack, err := scanServicePackRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
		}
		return domain.ServicePackDefinition{}, err
	}
	return pack, nil
}

func (s *Store) UpsertServicePack(ctx context.Context, pack domain.ServicePackDefinition) (domain.ServicePackDefinition, error) {
	normalized, err := normalizeServicePackTemplate(pack, 100)
	if err != nil {
		return domain.ServicePackDefinition{}, err
	}
	row := s.db.QueryRow(ctx, `insert into service_pack_templates(
		key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
		platform_notes_json,recommendations_json,components_json,status,source,version,display_order,created_at,updated_at
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),now())
	on conflict(key) do update set
		label=excluded.label,
		description=excluded.description,
		base_name_template=excluded.base_name_template,
		endpoint_hint=excluded.endpoint_hint,
		requires_endpoint_host=excluded.requires_endpoint_host,
		platform_notes_json=excluded.platform_notes_json,
		recommendations_json=excluded.recommendations_json,
		components_json=excluded.components_json,
		status=excluded.status,
		source=excluded.source,
		version=service_pack_templates.version + 1,
		display_order=excluded.display_order,
		updated_at=now()
	returning key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
		platform_notes_json,recommendations_json,components_json,status,source,version,display_order`,
		normalized.Key,
		normalized.Label,
		normalized.Description,
		normalized.BaseNameTemplate,
		normalized.EndpointHint,
		normalized.RequiresEndpointHost,
		mustJSON(normalized.PlatformNotes),
		mustJSON(normalized.Recommendations),
		mustJSON(normalized.Components),
		normalized.Status,
		normalized.Source,
		normalized.Version,
		normalized.DisplayOrder,
	)
	return scanServicePackRow(row)
}

func (s *Store) SetServicePackStatus(ctx context.Context, key string, status string) (domain.ServicePackDefinition, error) {
	key = strings.TrimSpace(key)
	status = strings.TrimSpace(status)
	if key == "" {
		return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
	}
	if !validServicePackStatus(status) {
		return domain.ServicePackDefinition{}, fmt.Errorf("invalid service pack status %q", status)
	}
	row := s.db.QueryRow(ctx, `update service_pack_templates
		set status=$2,version=version+1,updated_at=now()
		where key=$1
		returning key,label,description,base_name_template,endpoint_hint,requires_endpoint_host,
			platform_notes_json,recommendations_json,components_json,status,source,version,display_order`, key, status)
	pack, err := scanServicePackRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
		}
		return domain.ServicePackDefinition{}, err
	}
	return pack, nil
}

type servicePackCatalogKeyExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func repairServicePackTemplateKeys(ctx context.Context, exec servicePackCatalogKeyExecutor) error {
	if _, err := exec.Exec(ctx, `with ranked as (
	select ctid,
		row_number() over (
			partition by key
			order by
				case status when 'active' then 0 when 'disabled' then 1 else 2 end,
				case when source='default' then 1 else 0 end,
				version desc,
				updated_at desc,
				display_order asc,
				label asc
		) as rn
	from service_pack_templates
)
delete from service_pack_templates
where ctid in (select ctid from ranked where rn > 1)`); err != nil {
		return err
	}
	var hasUniqueKey bool
	if err := exec.QueryRow(ctx, `select exists(
	select 1
	from pg_index idx
	join pg_class tbl on tbl.oid = idx.indrelid
	join pg_namespace ns on ns.oid = tbl.relnamespace
	join pg_attribute attr on attr.attrelid = tbl.oid and attr.attnum = idx.indkey[0]
	where tbl.relname = 'service_pack_templates'
		and ns.nspname = current_schema()
		and idx.indisunique
		and idx.indnkeyatts = 1
		and attr.attname = 'key'
)`).Scan(&hasUniqueKey); err != nil {
		return err
	}
	if hasUniqueKey {
		return nil
	}
	_, err := exec.Exec(ctx, `create unique index if not exists idx_service_pack_templates_key_unique on service_pack_templates(key)`)
	return err
}

type servicePackScanner interface {
	Scan(dest ...any) error
}

func scanServicePackRow(row servicePackScanner) (domain.ServicePackDefinition, error) {
	var pack domain.ServicePackDefinition
	var platformNotesRaw []byte
	var recommendationsRaw []byte
	var componentsRaw []byte
	if err := row.Scan(
		&pack.Key,
		&pack.Label,
		&pack.Description,
		&pack.BaseNameTemplate,
		&pack.EndpointHint,
		&pack.RequiresEndpointHost,
		&platformNotesRaw,
		&recommendationsRaw,
		&componentsRaw,
		&pack.Status,
		&pack.Source,
		&pack.Version,
		&pack.DisplayOrder,
	); err != nil {
		return domain.ServicePackDefinition{}, err
	}
	if err := decodeServicePackJSON(platformNotesRaw, &pack.PlatformNotes); err != nil {
		return domain.ServicePackDefinition{}, err
	}
	if err := decodeServicePackJSON(recommendationsRaw, &pack.Recommendations); err != nil {
		return domain.ServicePackDefinition{}, err
	}
	if err := decodeServicePackJSON(componentsRaw, &pack.Components); err != nil {
		return domain.ServicePackDefinition{}, err
	}
	if pack.PlatformNotes == nil {
		pack.PlatformNotes = []string{}
	}
	if pack.Recommendations == nil {
		pack.Recommendations = []string{}
	}
	if pack.Components == nil {
		pack.Components = []domain.ServicePackComponent{}
	}
	return pack, nil
}

func decodeServicePackJSON(raw []byte, target any) error {
	if len(raw) == 0 {
		raw = []byte(`[]`)
	}
	return json.Unmarshal(raw, target)
}

func normalizeServicePackTemplate(pack domain.ServicePackDefinition, idx int) (domain.ServicePackDefinition, error) {
	pack.Key = normalizeServicePackKey(pack.Key)
	pack.Label = strings.TrimSpace(pack.Label)
	pack.Description = strings.TrimSpace(pack.Description)
	pack.BaseNameTemplate = strings.TrimSpace(pack.BaseNameTemplate)
	pack.EndpointHint = strings.TrimSpace(pack.EndpointHint)
	pack.Status = strings.TrimSpace(pack.Status)
	pack.Source = strings.TrimSpace(pack.Source)
	if pack.Label == "" {
		return domain.ServicePackDefinition{}, fmt.Errorf("service pack %q label is required", pack.Key)
	}
	if pack.Key == "" {
		pack.Key = normalizeServicePackKey(pack.Label)
	}
	if pack.Key == "" {
		return domain.ServicePackDefinition{}, fmt.Errorf("service pack key is required")
	}
	if len(pack.Components) == 0 {
		return domain.ServicePackDefinition{}, fmt.Errorf("service pack %q must define at least one component", pack.Key)
	}
	if pack.Status == "" {
		pack.Status = "active"
	}
	if !validServicePackStatus(pack.Status) {
		return domain.ServicePackDefinition{}, fmt.Errorf("invalid service pack %q status %q", pack.Key, pack.Status)
	}
	if pack.Source == "" {
		pack.Source = "default"
	}
	if pack.Version <= 0 {
		pack.Version = 1
	}
	if pack.DisplayOrder <= 0 {
		pack.DisplayOrder = (idx + 1) * 10
	}
	if pack.PlatformNotes == nil {
		pack.PlatformNotes = []string{}
	}
	if pack.Recommendations == nil {
		pack.Recommendations = []string{}
	}
	if pack.Components == nil {
		pack.Components = []domain.ServicePackComponent{}
	}
	for i := range pack.PlatformNotes {
		pack.PlatformNotes[i] = strings.TrimSpace(pack.PlatformNotes[i])
	}
	for i := range pack.Recommendations {
		pack.Recommendations[i] = strings.TrimSpace(pack.Recommendations[i])
	}
	for i := range pack.Components {
		component := &pack.Components[i]
		component.Label = strings.TrimSpace(component.Label)
		component.Description = strings.TrimSpace(component.Description)
		component.ServiceCode = normalizeInstanceRuntimeCode(component.ServiceCode)
		component.PresetKey = strings.TrimSpace(component.PresetKey)
		component.NameSuffix = strings.TrimSpace(component.NameSuffix)
		component.SlugSuffix = strings.TrimSpace(component.SlugSuffix)
		if component.ServiceCode == "" {
			return domain.ServicePackDefinition{}, fmt.Errorf("service pack %q component %d service_code is required", pack.Key, i+1)
		}
		if component.Label == "" {
			component.Label = component.ServiceCode
		}
		if component.Spec == nil {
			component.Spec = map[string]any{}
		}
	}
	return pack, nil
}

func normalizeServicePackKey(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastSeparator := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastSeparator = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSeparator = false
		default:
			if !lastSeparator {
				b.WriteByte('-')
				lastSeparator = true
			}
		}
		if b.Len() >= 64 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func validServicePackStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "active", "disabled", "deleted":
		return true
	default:
		return false
	}
}
