package http

import (
	"errors"
	nethttp "net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
)

type nodeSSHAccessMethodCreateRequest struct {
	SSHHost          string `json:"ssh_host"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHHostKeySHA256 string `json:"ssh_host_key_sha256"`
	PrivateKey       string `json:"private_key"`
	IsEnabled        *bool  `json:"is_enabled"`
}

type nodeSSHAccessMethodResponse struct {
	ID               string    `json:"id"`
	NodeID           string    `json:"node_id"`
	Method           string    `json:"method"`
	IsEnabled        bool      `json:"is_enabled"`
	SSHHost          string    `json:"ssh_host"`
	SSHPort          int       `json:"ssh_port"`
	SSHUser          string    `json:"ssh_user"`
	SSHHostKeySHA256 string    `json:"ssh_host_key_sha256"`
	AuthType         string    `json:"auth_type"`
	SecretConfigured bool      `json:"secret_configured"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (s *Server) createNodeSSHAccessMethod(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeSSHAccessMethodCreateRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid ssh access method payload")
		return
	}
	defer func() {
		req.PrivateKey = ""
	}()

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	input := domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          req.SSHHost,
		SSHPort:          req.SSHPort,
		SSHUser:          req.SSHUser,
		SSHHostKeySHA256: req.SSHHostKeySHA256,
		PrivateKey:       req.PrivateKey,
		IsEnabled:        isEnabled,
	}
	if authCtx, ok := authFromRequest(r); ok {
		input.ActorUserID = &authCtx.User.ID
	}
	created, err := s.store.CreateNodeSSHAccessMethod(r.Context(), idParam(r), input)
	if err != nil {
		writeNodeSSHAccessMethodCreateError(w, err)
		return
	}
	writeJSON(w, 201, redactedNodeSSHAccessMethod(created))
}

func writeNodeSSHAccessMethodCreateError(w nethttp.ResponseWriter, err error) {
	var validationErr domain.ValidationError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeErr(w, 404, "node not found")
	case errors.Is(err, domain.ErrNodeSSHAccessMethodDuplicate), errors.Is(err, domain.ErrNodeNotManageable):
		writeErr(w, 409, err.Error())
	case errors.Is(err, postgres.ErrSecretServiceUnavailable):
		writeErr(w, 503, "secret storage is not configured")
	case errors.As(err, &validationErr):
		writeErr(w, 400, validationErr.Error())
	default:
		writeErr(w, 500, "ssh access method create failed")
	}
}

func redactedNodeSSHAccessMethod(method domain.NodeAccessMethod) nodeSSHAccessMethodResponse {
	return nodeSSHAccessMethodResponse{
		ID:               method.ID,
		NodeID:           method.NodeID,
		Method:           method.Method,
		IsEnabled:        method.IsEnabled,
		SSHHost:          method.SSHHost,
		SSHPort:          method.SSHPort,
		SSHUser:          method.SSHUser,
		SSHHostKeySHA256: method.SSHHostKeySHA256,
		AuthType:         method.AuthType,
		SecretConfigured: method.SecretRefID != nil,
		CreatedAt:        method.CreatedAt,
		UpdatedAt:        method.UpdatedAt,
	}
}
