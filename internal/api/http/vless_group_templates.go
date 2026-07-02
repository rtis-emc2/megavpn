package http

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/rbac"
)

type vlessGroupTemplateCatalogStore interface {
	EnsureDefaultVLESSGroupTemplates(context.Context, []domain.VLESSGroupTemplate) error
	ListVLESSGroupTemplates(context.Context) ([]domain.VLESSGroupTemplate, error)
}

type vlessGroupTemplateCatalogManageStore interface {
	vlessGroupTemplateCatalogStore
	ListVLESSGroupTemplateCatalog(context.Context) ([]domain.VLESSGroupTemplate, error)
	UpsertVLESSGroupTemplate(context.Context, domain.VLESSGroupTemplate) (domain.VLESSGroupTemplate, error)
	SetVLESSGroupTemplateStatus(context.Context, string, string) (domain.VLESSGroupTemplate, error)
}

func defaultVLESSGroupTemplates() []domain.VLESSGroupTemplate {
	return []domain.VLESSGroupTemplate{
		{
			Key:          "default",
			Label:        "Default access",
			Description:  "Use the VLESS instance default route. If the instance uses managed egress, this group follows that route.",
			AccessMode:   "instance_default",
			EgressMode:   "default",
			OutboundTag:  "direct",
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 10,
		},
		{
			Key:          "current_node_exit",
			Label:        "Current node exit",
			Description:  "Force traffic to exit from the same node that accepts the VLESS connection.",
			AccessMode:   "local_breakout",
			EgressMode:   "local_breakout",
			OutboundTag:  "direct",
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 20,
		},
		{
			Key:         "default_ads_blocked",
			Label:       "Default access with ad blocking",
			Description: "Use the instance default route and block managed advertising domains before final outbound selection.",
			AccessMode:  "instance_default",
			EgressMode:  "default",
			OutboundTag: "direct",
			AdBlock:     true,
			Rules: []map[string]any{
				{
					"type":         "field",
					"domain":       []string{"geosite:category-ads-all"},
					"outbound_tag": "block",
				},
			},
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 30,
		},
		{
			Key:          "blocked",
			Label:        "Blocked",
			Description:  "Deny all traffic for clients assigned to this group.",
			AccessMode:   "block",
			EgressMode:   "block",
			OutboundTag:  "block",
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 90,
		},
	}
}

func DefaultVLESSGroupTemplates() []domain.VLESSGroupTemplate {
	defaults := defaultVLESSGroupTemplates()
	out := make([]domain.VLESSGroupTemplate, len(defaults))
	for i, template := range defaults {
		out[i] = template
		out[i].Rules = cloneRulesHTTP(template.Rules)
		out[i].ExtraRules = cloneRulesHTTP(template.ExtraRules)
	}
	return out
}

func cloneRulesHTTP(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, len(in))
	for i, item := range in {
		out[i] = cloneMapHTTP(item)
	}
	return out
}

func (s *Server) listVLESSGroupTemplates(w nethttp.ResponseWriter, r *nethttp.Request) {
	if truthyQuery(r, "include_inactive") {
		authCtx, ok := authFromRequest(r)
		if !ok || !rbac.HasPermission(authCtx.PermissionCodes, "settings.manage") {
			writeErr(w, 403, "settings.manage permission is required")
			return
		}
		catalog, ok := s.store.(vlessGroupTemplateCatalogManageStore)
		if !ok {
			writeErr(w, 501, "vless group catalog management is not supported")
			return
		}
		if err := catalog.EnsureDefaultVLESSGroupTemplates(r.Context(), DefaultVLESSGroupTemplates()); err != nil {
			if isVLESSGroupTemplateCatalogUnavailable(err) {
				writeJSON(w, 200, DefaultVLESSGroupTemplates())
				return
			}
			writeErr(w, 500, err.Error())
			return
		}
		templates, err := catalog.ListVLESSGroupTemplateCatalog(r.Context())
		if err != nil {
			if isVLESSGroupTemplateCatalogUnavailable(err) {
				writeJSON(w, 200, DefaultVLESSGroupTemplates())
				return
			}
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, templates)
		return
	}
	templates, err := s.availableVLESSGroupTemplates(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, templates)
}

func (s *Server) upsertVLESSGroupTemplate(w nethttp.ResponseWriter, r *nethttp.Request) {
	catalog, ok := s.store.(vlessGroupTemplateCatalogManageStore)
	if !ok {
		writeErr(w, 501, "vless group catalog management is not supported")
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	var template domain.VLESSGroupTemplate
	if !decode(r, &template) {
		writeErr(w, 400, "invalid vless group payload")
		return
	}
	if key != "" {
		template.Key = key
	}
	if strings.TrimSpace(template.Source) == "" {
		template.Source = "operator"
	}
	if strings.TrimSpace(template.Status) == "" {
		template.Status = "active"
	}
	updated, err := catalog.UpsertVLESSGroupTemplate(r.Context(), template)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "vless_group.upsert", "vless_group", nil, "vless group template upserted: "+updated.Key)
	}
	writeJSON(w, 200, updated)
}

func (s *Server) setVLESSGroupTemplateStatus(status string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		catalog, ok := s.store.(vlessGroupTemplateCatalogManageStore)
		if !ok {
			writeErr(w, 501, "vless group catalog management is not supported")
			return
		}
		key := strings.TrimSpace(r.PathValue("key"))
		updated, err := catalog.SetVLESSGroupTemplateStatus(r.Context(), key, status)
		if errors.Is(err, domain.ErrVLESSGroupTemplateNotFound) {
			writeErr(w, 404, "vless group template not found")
			return
		}
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if authCtx, ok := authFromRequest(r); ok {
			_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "vless_group."+status, "vless_group", nil, "vless group template "+status+": "+updated.Key)
		}
		writeJSON(w, 200, updated)
	}
}

func (s *Server) availableVLESSGroupTemplates(ctx context.Context) ([]domain.VLESSGroupTemplate, error) {
	catalog, ok := s.store.(vlessGroupTemplateCatalogStore)
	defaults := DefaultVLESSGroupTemplates()
	if !ok {
		return defaults, nil
	}
	if err := catalog.EnsureDefaultVLESSGroupTemplates(ctx, defaults); err != nil {
		if isVLESSGroupTemplateCatalogUnavailable(err) {
			return defaults, nil
		}
		return nil, err
	}
	templates, err := catalog.ListVLESSGroupTemplates(ctx)
	if isVLESSGroupTemplateCatalogUnavailable(err) {
		return defaults, nil
	}
	return templates, err
}

func vlessGroupTemplatesAsSpec(templates []domain.VLESSGroupTemplate) []any {
	out := make([]any, 0, len(templates))
	for _, template := range templates {
		if strings.TrimSpace(template.Key) == "" || strings.TrimSpace(template.Status) == "deleted" {
			continue
		}
		group := map[string]any{
			"key":          template.Key,
			"label":        template.Label,
			"access_mode":  template.AccessMode,
			"egress_mode":  template.EgressMode,
			"outbound_tag": template.OutboundTag,
		}
		if template.Description != "" {
			group["description"] = template.Description
		}
		if template.EgressNodeID != "" {
			group["egress_node_id"] = template.EgressNodeID
		}
		if template.TargetInstanceID != "" {
			group["target_instance_id"] = template.TargetInstanceID
		}
		if template.AdBlock {
			group["ad_block"] = true
		}
		rules := cloneRulesHTTP(template.Rules)
		if len(template.ExtraRules) > 0 {
			rules = append(rules, cloneRulesHTTP(template.ExtraRules)...)
			group["extra_rules"] = cloneRulesHTTP(template.ExtraRules)
		}
		if len(rules) > 0 {
			group["rules"] = rules
		}
		out = append(out, group)
	}
	if len(out) == 0 {
		out = append(out, map[string]any{
			"key":          "default",
			"label":        "Default access",
			"access_mode":  "instance_default",
			"egress_mode":  "default",
			"outbound_tag": "direct",
		})
	}
	return out
}

func ensureVLESSDefaultGroup(spec map[string]any, templates []domain.VLESSGroupTemplate) {
	if spec == nil {
		return
	}
	groups := vlessGroupTemplatesAsSpec(templates)
	spec["vless_groups"] = groups
	current := strings.TrimSpace(firstStringHTTP(spec["default_vless_group"], spec["default_xray_group"], spec["default_outbound_group"]))
	if current != "" {
		for _, item := range groups {
			group, _ := item.(map[string]any)
			if firstStringHTTP(group["key"]) == current {
				spec["default_vless_group"] = current
				return
			}
		}
	}
	for _, item := range groups {
		group, _ := item.(map[string]any)
		if key := firstStringHTTP(group["key"]); key != "" {
			spec["default_vless_group"] = key
			return
		}
	}
	spec["default_vless_group"] = "default"
}

func isVLESSGroupTemplateCatalogUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42P01", "42703":
			return true
		}
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "vless_group_templates") && (strings.Contains(text, "does not exist") || strings.Contains(text, "undefined"))
}
