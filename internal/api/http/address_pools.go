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

type addressPoolStore interface {
	EnsureDefaultAddressPoolSpaces(context.Context) error
	AddressPoolInventory(context.Context) (domain.AddressPoolInventory, error)
	CreateAddressPoolSpace(context.Context, domain.AddressPoolSpace) (domain.AddressPoolSpace, error)
	UpdateAddressPoolSpace(context.Context, string, domain.AddressPoolSpace) (domain.AddressPoolSpace, error)
	DeleteAddressPoolSpace(context.Context, string) (domain.AddressPoolSpace, error)
	SetAddressPoolRouting(context.Context, string, bool) (domain.AddressPoolSpace, error)
}

type addressPoolRoutingRequest struct {
	RoutingEnabled bool `json:"routing_enabled"`
}

func (s *Server) listAddressPools(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(addressPoolStore)
	if !ok {
		writeErr(w, 501, "address pool catalog is not supported")
		return
	}
	inventory, err := store.AddressPoolInventory(r.Context())
	if err != nil {
		if isAddressPoolCatalogUnavailableHTTP(err) {
			writeJSON(w, 200, domain.AddressPoolInventory{Spaces: []domain.AddressPoolSpace{}, Allocations: []domain.AddressPoolAllocation{}})
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, inventory)
}

func isAddressPoolCatalogUnavailableHTTP(err error) bool {
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
	return strings.Contains(text, "address_pool_") && (strings.Contains(text, "does not exist") || strings.Contains(text, "undefined"))
}

func (s *Server) createAddressPoolSpace(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(addressPoolStore)
	if !ok {
		writeErr(w, 501, "address pool catalog is not supported")
		return
	}
	var req domain.AddressPoolSpace
	if !decode(r, &req) {
		writeErr(w, 400, "invalid address pool payload")
		return
	}
	created, err := store.CreateAddressPoolSpace(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "address_pool.create", "address_pool", &created.ID, "address pool space created: "+created.Key)
	}
	writeJSON(w, 201, created)
}

func (s *Server) updateAddressPoolSpace(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(addressPoolStore)
	if !ok {
		writeErr(w, 501, "address pool catalog is not supported")
		return
	}
	var req domain.AddressPoolSpace
	if !decode(r, &req) {
		writeErr(w, 400, "invalid address pool payload")
		return
	}
	poolID := strings.TrimSpace(r.PathValue("id"))
	updated, err := store.UpdateAddressPoolSpace(r.Context(), poolID, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "address_pool.update", "address_pool", &updated.ID, "address pool space updated: "+updated.Key)
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteAddressPoolSpace(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(addressPoolStore)
	if !ok {
		writeErr(w, 501, "address pool catalog is not supported")
		return
	}
	poolID := strings.TrimSpace(r.PathValue("id"))
	deleted, err := store.DeleteAddressPoolSpace(r.Context(), poolID)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "address_pool.delete", "address_pool", &deleted.ID, "address pool space deleted: "+deleted.Key)
	}
	writeJSON(w, 200, deleted)
}

func (s *Server) setAddressPoolRouting(w nethttp.ResponseWriter, r *nethttp.Request) {
	store, ok := s.store.(addressPoolStore)
	if !ok {
		writeErr(w, 501, "address pool catalog is not supported")
		return
	}
	var req addressPoolRoutingRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid address pool routing payload")
		return
	}
	poolID := strings.TrimSpace(r.PathValue("id"))
	updated, err := store.SetAddressPoolRouting(r.Context(), poolID, req.RoutingEnabled)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "address_pool.routing", "address_pool", &updated.ID, "address pool routing updated: "+updated.Key)
	}
	writeJSON(w, 200, updated)
}
