package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) EnsureDefaultVLESSGroupTemplates(ctx context.Context, templates []domain.VLESSGroupTemplate) error {
	if len(templates) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for idx, template := range templates {
		normalized, err := normalizeVLESSGroupTemplate(template, idx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into vless_group_templates(
			key,label,description,access_mode,egress_mode,egress_node_id,target_instance_id,
			outbound_tag,ad_block,rules_json,extra_rules_json,status,source,version,display_order,created_at,updated_at
		) values($1,$2,$3,$4,$5,nullif($6,'')::uuid,nullif($7,'')::uuid,$8,$9,$10,$11,$12,$13,$14,$15,now(),now())
		on conflict(key) do update set
			label=excluded.label,
			description=excluded.description,
			access_mode=excluded.access_mode,
			egress_mode=excluded.egress_mode,
			outbound_tag=excluded.outbound_tag,
			ad_block=excluded.ad_block,
			rules_json=excluded.rules_json,
			extra_rules_json=excluded.extra_rules_json,
			status=case
				when vless_group_templates.status in ('disabled','deleted') then vless_group_templates.status
				else excluded.status
			end,
			version=excluded.version,
			display_order=excluded.display_order,
			updated_at=now()
		where vless_group_templates.source='default'`,
			normalized.Key,
			normalized.Label,
			normalized.Description,
			normalized.AccessMode,
			normalized.EgressMode,
			normalized.EgressNodeID,
			normalized.TargetInstanceID,
			normalized.OutboundTag,
			normalized.AdBlock,
			mustJSON(normalized.Rules),
			mustJSON(normalized.ExtraRules),
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

func (s *Store) ListVLESSGroupTemplates(ctx context.Context) ([]domain.VLESSGroupTemplate, error) {
	rows, err := s.db.Query(ctx, `select key,label,description,access_mode,egress_mode,
		coalesce(egress_node_id::text,''),coalesce(target_instance_id::text,''),outbound_tag,ad_block,
		rules_json,extra_rules_json,status,source,version,display_order
		from vless_group_templates
		where status='active'
		order by display_order asc,label asc,key asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVLESSGroupTemplateRows(rows)
}

func (s *Store) ListVLESSGroupTemplateCatalog(ctx context.Context) ([]domain.VLESSGroupTemplate, error) {
	rows, err := s.db.Query(ctx, `select key,label,description,access_mode,egress_mode,
		coalesce(egress_node_id::text,''),coalesce(target_instance_id::text,''),outbound_tag,ad_block,
		rules_json,extra_rules_json,status,source,version,display_order
		from vless_group_templates
		order by case status when 'active' then 0 when 'disabled' then 1 else 2 end, display_order asc,label asc,key asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVLESSGroupTemplateRows(rows)
}

func (s *Store) UpsertVLESSGroupTemplate(ctx context.Context, template domain.VLESSGroupTemplate) (domain.VLESSGroupTemplate, error) {
	normalized, err := normalizeVLESSGroupTemplate(template, 100)
	if err != nil {
		return domain.VLESSGroupTemplate{}, err
	}
	row := s.db.QueryRow(ctx, `insert into vless_group_templates(
		key,label,description,access_mode,egress_mode,egress_node_id,target_instance_id,
		outbound_tag,ad_block,rules_json,extra_rules_json,status,source,version,display_order,created_at,updated_at
	) values($1,$2,$3,$4,$5,nullif($6,'')::uuid,nullif($7,'')::uuid,$8,$9,$10,$11,$12,$13,$14,$15,now(),now())
	on conflict(key) do update set
		label=excluded.label,
		description=excluded.description,
		access_mode=excluded.access_mode,
		egress_mode=excluded.egress_mode,
		egress_node_id=excluded.egress_node_id,
		target_instance_id=excluded.target_instance_id,
		outbound_tag=excluded.outbound_tag,
		ad_block=excluded.ad_block,
		rules_json=excluded.rules_json,
		extra_rules_json=excluded.extra_rules_json,
		status=excluded.status,
		source=excluded.source,
		version=vless_group_templates.version+1,
		display_order=excluded.display_order,
		updated_at=now()
	returning key,label,description,access_mode,egress_mode,
		coalesce(egress_node_id::text,''),coalesce(target_instance_id::text,''),outbound_tag,ad_block,
		rules_json,extra_rules_json,status,source,version,display_order`,
		normalized.Key,
		normalized.Label,
		normalized.Description,
		normalized.AccessMode,
		normalized.EgressMode,
		normalized.EgressNodeID,
		normalized.TargetInstanceID,
		normalized.OutboundTag,
		normalized.AdBlock,
		mustJSON(normalized.Rules),
		mustJSON(normalized.ExtraRules),
		normalized.Status,
		normalized.Source,
		normalized.Version,
		normalized.DisplayOrder,
	)
	return scanVLESSGroupTemplate(row)
}

func (s *Store) SetVLESSGroupTemplateStatus(ctx context.Context, key string, status string) (domain.VLESSGroupTemplate, error) {
	key = strings.TrimSpace(key)
	status = strings.TrimSpace(status)
	if key == "" {
		return domain.VLESSGroupTemplate{}, domain.ErrVLESSGroupTemplateNotFound
	}
	if !validVLESSGroupTemplateStatus(status) {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("invalid vless group status %q", status)
	}
	row := s.db.QueryRow(ctx, `update vless_group_templates
		set status=$2,version=version+1,updated_at=now()
		where key=$1
		returning key,label,description,access_mode,egress_mode,
			coalesce(egress_node_id::text,''),coalesce(target_instance_id::text,''),outbound_tag,ad_block,
			rules_json,extra_rules_json,status,source,version,display_order`, key, status)
	template, err := scanVLESSGroupTemplate(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.VLESSGroupTemplate{}, domain.ErrVLESSGroupTemplateNotFound
		}
		return domain.VLESSGroupTemplate{}, err
	}
	return template, nil
}

type vlessGroupTemplateScanner interface {
	Scan(dest ...any) error
}

type vlessGroupTemplateRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanVLESSGroupTemplateRows(rows vlessGroupTemplateRows) ([]domain.VLESSGroupTemplate, error) {
	var out []domain.VLESSGroupTemplate
	for rows.Next() {
		template, err := scanVLESSGroupTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, template)
	}
	return out, rows.Err()
}

func scanVLESSGroupTemplate(row vlessGroupTemplateScanner) (domain.VLESSGroupTemplate, error) {
	var template domain.VLESSGroupTemplate
	var rulesRaw []byte
	var extraRulesRaw []byte
	if err := row.Scan(
		&template.Key,
		&template.Label,
		&template.Description,
		&template.AccessMode,
		&template.EgressMode,
		&template.EgressNodeID,
		&template.TargetInstanceID,
		&template.OutboundTag,
		&template.AdBlock,
		&rulesRaw,
		&extraRulesRaw,
		&template.Status,
		&template.Source,
		&template.Version,
		&template.DisplayOrder,
	); err != nil {
		return domain.VLESSGroupTemplate{}, err
	}
	if err := decodeVLESSGroupTemplateJSON(rulesRaw, &template.Rules); err != nil {
		return domain.VLESSGroupTemplate{}, err
	}
	if err := decodeVLESSGroupTemplateJSON(extraRulesRaw, &template.ExtraRules); err != nil {
		return domain.VLESSGroupTemplate{}, err
	}
	if template.Rules == nil {
		template.Rules = []map[string]any{}
	}
	if template.ExtraRules == nil {
		template.ExtraRules = []map[string]any{}
	}
	return template, nil
}

func decodeVLESSGroupTemplateJSON(raw []byte, target any) error {
	if len(raw) == 0 {
		raw = []byte(`[]`)
	}
	return json.Unmarshal(raw, target)
}

func normalizeVLESSGroupTemplate(template domain.VLESSGroupTemplate, idx int) (domain.VLESSGroupTemplate, error) {
	template.Key = normalizeVLESSGroupTemplateKey(template.Key)
	template.Label = strings.TrimSpace(template.Label)
	template.Description = strings.TrimSpace(template.Description)
	template.AccessMode = normalizeVLESSGroupAccessMode(template.AccessMode)
	template.EgressMode = normalizeVLESSGroupEgressMode(template.EgressMode, template.AccessMode)
	template.EgressNodeID = strings.TrimSpace(template.EgressNodeID)
	template.TargetInstanceID = strings.TrimSpace(template.TargetInstanceID)
	template.OutboundTag = normalizeVLESSGroupOutboundTag(template.OutboundTag)
	template.Status = strings.TrimSpace(template.Status)
	template.Source = strings.TrimSpace(template.Source)
	if template.AccessMode == "block" || template.AccessMode == "instance_only" {
		template.OutboundTag = "block"
	}
	if template.Key == "" {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("vless group key is required")
	}
	if template.Label == "" {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("vless group %q label is required", template.Key)
	}
	if template.Status == "" {
		template.Status = "active"
	}
	if !validVLESSGroupTemplateStatus(template.Status) {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("invalid vless group %q status %q", template.Key, template.Status)
	}
	if template.Source == "" {
		template.Source = "operator"
	}
	if template.Version <= 0 {
		template.Version = 1
	}
	if template.DisplayOrder <= 0 {
		template.DisplayOrder = (idx + 1) * 10
	}
	if template.AccessMode == "egress_node" && template.EgressNodeID == "" {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("vless group %q requires egress_node_id", template.Key)
	}
	if template.AccessMode == "instance_only" && template.TargetInstanceID == "" && len(template.Rules) == 0 {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("vless group %q requires target_instance_id or rules", template.Key)
	}
	if template.Rules == nil {
		template.Rules = []map[string]any{}
	}
	if template.ExtraRules == nil {
		template.ExtraRules = []map[string]any{}
	}
	return template, nil
}

func normalizeVLESSGroupTemplateKey(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == ':':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	out := strings.Trim(b.String(), "_.:-")
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func normalizeVLESSGroupAccessMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default", "auto", "inherit", "instance_default":
		return "instance_default"
	case "local", "direct", "current_node", "local_breakout":
		return "local_breakout"
	case "remote_node", "remote_egress", "node", "egress_node":
		return "egress_node"
	case "target_instance", "allow_instance", "instance_only":
		return "instance_only"
	case "deny", "blocked", "block":
		return "block"
	default:
		return "instance_default"
	}
}

func normalizeVLESSGroupEgressMode(raw string, accessMode string) string {
	mode := normalizeVLESSGroupAccessMode(raw)
	if strings.TrimSpace(raw) == "" {
		mode = accessMode
	}
	if mode == "instance_default" {
		return "default"
	}
	return mode
}

func normalizeVLESSGroupOutboundTag(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "direct"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == ':':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	if b.Len() == 0 {
		return "direct"
	}
	return b.String()
}

func validVLESSGroupTemplateStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "active", "disabled", "deleted":
		return true
	default:
		return false
	}
}
