package http

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

type firewallStore interface {
	FirewallInventory(context.Context) (domain.FirewallInventory, error)
	CreateFirewallPolicy(context.Context, domain.FirewallPolicy) (domain.FirewallPolicy, error)
	UpdateFirewallPolicy(context.Context, string, domain.FirewallPolicy) (domain.FirewallPolicy, error)
	DeleteFirewallPolicy(context.Context, string) (domain.FirewallPolicy, error)
	CreateFirewallAddressList(context.Context, domain.FirewallAddressList) (domain.FirewallAddressList, error)
	UpdateFirewallAddressList(context.Context, string, domain.FirewallAddressList) (domain.FirewallAddressList, error)
	DeleteFirewallAddressList(context.Context, string) (domain.FirewallAddressList, error)
	CreateFirewallAddressEntry(context.Context, string, domain.FirewallAddressEntry) (domain.FirewallAddressEntry, error)
	UpdateFirewallAddressEntry(context.Context, string, string, domain.FirewallAddressEntry) (domain.FirewallAddressEntry, error)
	DeleteFirewallAddressEntry(context.Context, string, string) (domain.FirewallAddressEntry, error)
	CreateFirewallRule(context.Context, string, domain.FirewallRule) (domain.FirewallRule, error)
	UpdateFirewallRule(context.Context, string, string, domain.FirewallRule) (domain.FirewallRule, error)
	DeleteFirewallRule(context.Context, string, string) (domain.FirewallRule, error)
	CreateFirewallPreviewJob(context.Context, string, string, bool) (domain.Job, error)
	CreateFirewallApplyJob(context.Context, string, string, bool) (domain.Job, error)
}

type firewallApplyRequest struct {
	PolicyID             string `json:"policy_id"`
	EnforceDefaultPolicy bool   `json:"enforce_default_policy"`
}

func (s *Server) listFirewallInventory(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	inventory, err := store.FirewallInventory(r.Context())
	if err != nil {
		if isFirewallCatalogUnavailableHTTP(err) {
			writeJSON(w, 200, domain.FirewallInventory{
				AddressLists: []domain.FirewallAddressList{},
				Entries:      []domain.FirewallAddressEntry{},
				Policies:     []domain.FirewallPolicy{},
				Rules:        []domain.FirewallRule{},
				NodeStates:   []domain.FirewallNodeState{},
			})
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, inventory)
}

func (s *Server) createFirewallPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	var req domain.FirewallPolicy
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall policy payload")
		return
	}
	created, err := store.CreateFirewallPolicy(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.policy.create", "firewall", &created.ID, "firewall policy created: "+created.Key)
	}
	writeJSON(w, 201, created)
}

func (s *Server) updateFirewallPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	var req domain.FirewallPolicy
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall policy payload")
		return
	}
	updated, err := store.UpdateFirewallPolicy(r.Context(), policyID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.policy.update", "firewall", &updated.ID, "firewall policy updated: "+updated.Key)
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteFirewallPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	deleted, err := store.DeleteFirewallPolicy(r.Context(), policyID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.policy.delete", "firewall", &deleted.ID, "firewall policy deleted: "+deleted.Key)
	}
	writeJSON(w, 200, deleted)
}

func (s *Server) createFirewallAddressList(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	var req domain.FirewallAddressList
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall address-list payload")
		return
	}
	created, err := store.CreateFirewallAddressList(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.address_list.create", "firewall", &created.ID, "firewall address-list created: "+created.Key)
	}
	writeJSON(w, 201, created)
}

func (s *Server) updateFirewallAddressList(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	listID := strings.TrimSpace(r.PathValue("id"))
	var req domain.FirewallAddressList
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall address-list payload")
		return
	}
	updated, err := store.UpdateFirewallAddressList(r.Context(), listID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.address_list.update", "firewall", &updated.ID, "firewall address-list updated: "+updated.Key)
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteFirewallAddressList(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	listID := strings.TrimSpace(r.PathValue("id"))
	deleted, err := store.DeleteFirewallAddressList(r.Context(), listID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.address_list.delete", "firewall", &deleted.ID, "firewall address-list deleted: "+deleted.Key)
	}
	writeJSON(w, 200, deleted)
}

func (s *Server) createFirewallAddressEntry(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	listID := strings.TrimSpace(r.PathValue("id"))
	var req domain.FirewallAddressEntry
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall address entry payload")
		return
	}
	created, err := store.CreateFirewallAddressEntry(r.Context(), listID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.address_entry.create", "firewall", &created.ID, "firewall address entry created")
	}
	writeJSON(w, 201, created)
}

func (s *Server) updateFirewallAddressEntry(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	listID := strings.TrimSpace(r.PathValue("id"))
	entryID := strings.TrimSpace(r.PathValue("entry_id"))
	var req domain.FirewallAddressEntry
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall address entry payload")
		return
	}
	updated, err := store.UpdateFirewallAddressEntry(r.Context(), listID, entryID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.address_entry.update", "firewall", &updated.ID, "firewall address entry updated")
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteFirewallAddressEntry(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	listID := strings.TrimSpace(r.PathValue("id"))
	entryID := strings.TrimSpace(r.PathValue("entry_id"))
	deleted, err := store.DeleteFirewallAddressEntry(r.Context(), listID, entryID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.address_entry.delete", "firewall", &deleted.ID, "firewall address entry deleted")
	}
	writeJSON(w, 200, deleted)
}

func (s *Server) createFirewallRule(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	var req domain.FirewallRule
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall rule payload")
		return
	}
	created, err := store.CreateFirewallRule(r.Context(), policyID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.rule.create", "firewall", &created.ID, "firewall rule created")
	}
	writeJSON(w, 201, created)
}

func (s *Server) updateFirewallRule(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	ruleID := strings.TrimSpace(r.PathValue("rule_id"))
	var req domain.FirewallRule
	if !decode(r, &req) {
		writeErr(w, 400, "invalid firewall rule payload")
		return
	}
	updated, err := store.UpdateFirewallRule(r.Context(), policyID, ruleID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.rule.update", "firewall", &updated.ID, "firewall rule updated")
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteFirewallRule(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	policyID := strings.TrimSpace(r.PathValue("id"))
	ruleID := strings.TrimSpace(r.PathValue("rule_id"))
	deleted, err := store.DeleteFirewallRule(r.Context(), policyID, ruleID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.rule.delete", "firewall", &deleted.ID, "firewall rule deleted")
	}
	writeJSON(w, 200, deleted)
}

func (s *Server) applyNodeFirewallPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	nodeID := strings.TrimSpace(r.PathValue("id"))
	var req firewallApplyRequest
	if r.Body != nil && r.ContentLength != 0 {
		if !decode(r, &req) {
			writeErr(w, 400, "invalid firewall apply payload")
			return
		}
	}
	job, err := store.CreateFirewallApplyJob(r.Context(), nodeID, req.PolicyID, req.EnforceDefaultPolicy)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.apply", "node", &nodeID, "firewall apply queued")
	}
	writeJSON(w, 202, job)
}

func (s *Server) previewNodeFirewallPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(firewallStore)
	if !ok {
		writeErr(w, 501, "firewall catalog is not supported")
		return
	}
	nodeID := strings.TrimSpace(r.PathValue("id"))
	var req firewallApplyRequest
	if r.Body != nil && r.ContentLength != 0 {
		if !decode(r, &req) {
			writeErr(w, 400, "invalid firewall preview payload")
			return
		}
	}
	job, err := store.CreateFirewallPreviewJob(r.Context(), nodeID, req.PolicyID, req.EnforceDefaultPolicy)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "firewall.preview", "node", &nodeID, "firewall preview queued")
	}
	writeJSON(w, 202, job)
}

func isFirewallCatalogUnavailableHTTP(err error) bool {
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
	text := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(text, "firewall_") && (strings.Contains(text, "does not exist") || strings.Contains(text, "undefined"))
}
