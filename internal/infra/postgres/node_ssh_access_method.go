package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"golang.org/x/crypto/ssh"
)

const maxNodeSSHPrivateKeyBytes = 256 * 1024

type auditInsertExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *Store) CreateNodeSSHAccessMethod(ctx context.Context, nodeID string, input domain.NodeSSHAccessMethodCreateInput) (domain.NodeAccessMethod, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.NodeAccessMethod{}, domain.ValidationError{Message: "node id is required"}
	}
	input, privateKey, err := normalizeNodeSSHAccessMethodCreateInput(input)
	if err != nil {
		return domain.NodeAccessMethod{}, err
	}
	defer zeroBytes(privateKey)

	now := time.Now().UTC()
	method := domain.NodeAccessMethod{
		ID:               id.New(),
		NodeID:           nodeID,
		Method:           "ssh",
		IsEnabled:        input.IsEnabled,
		SSHHost:          input.SSHHost,
		SSHPort:          input.SSHPort,
		SSHUser:          input.SSHUser,
		SSHHostKeySHA256: input.SSHHostKeySHA256,
		AuthType:         "ssh_key",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	secretMeta := map[string]any{
		"purpose":          "node_ssh_bootstrap",
		"node_id":          nodeID,
		"access_method_id": method.ID,
		"auth_type":        "ssh_key",
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.NodeAccessMethod{}, err
	}
	defer tx.Rollback(ctx)

	lockScope := strings.ToLower(input.SSHHost) + ":" + strconv.Itoa(input.SSHPort) + ":" + strings.ToLower(input.SSHUser)
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext($1), hashtext($2))`, "node_ssh_access:"+nodeID, lockScope); err != nil {
		return domain.NodeAccessMethod{}, err
	}

	var status string
	if err := tx.QueryRow(ctx, `select status from nodes where id=$1 for update`, nodeID).Scan(&status); err != nil {
		return domain.NodeAccessMethod{}, err
	}
	if strings.EqualFold(strings.TrimSpace(status), "retired") {
		return domain.NodeAccessMethod{}, domain.ErrNodeNotManageable
	}

	var duplicate bool
	if err := tx.QueryRow(ctx, `
select exists(
	select 1 from node_access_methods
	where node_id=$1
	  and method='ssh'
	  and lower(coalesce(ssh_host,''))=lower($2)
	  and coalesce(ssh_port,22)=$3
	  and lower(coalesce(ssh_user,''))=lower($4)
)`, nodeID, input.SSHHost, input.SSHPort, input.SSHUser).Scan(&duplicate); err != nil {
		return domain.NodeAccessMethod{}, err
	}
	if duplicate {
		return domain.NodeAccessMethod{}, domain.ErrNodeSSHAccessMethodDuplicate
	}

	preparedSecret, err := s.prepareSecretRef("ssh_key", privateKey, secretMeta)
	if err != nil {
		return domain.NodeAccessMethod{}, err
	}
	secretID := preparedSecret.ref.ID
	method.SecretRefID = &secretID

	if err := insertPreparedSecretRef(ctx, tx, preparedSecret); err != nil {
		return domain.NodeAccessMethod{}, err
	}
	if _, err := tx.Exec(ctx, `insert into node_access_methods(id,node_id,method,is_enabled,ssh_host,ssh_port,ssh_user,ssh_host_key_sha256,auth_type,secret_ref_id,created_at,updated_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		method.ID, method.NodeID, method.Method, method.IsEnabled, method.SSHHost, method.SSHPort, method.SSHUser, method.SSHHostKeySHA256, method.AuthType, method.SecretRefID, method.CreatedAt, method.UpdatedAt); err != nil {
		return domain.NodeAccessMethod{}, err
	}
	if _, err := insertAuditForUser(ctx, tx, input.ActorUserID, "node.ssh_access_method.create", "node", &nodeID, fmt.Sprintf("ssh access method created: %s", method.ID)); err != nil {
		return domain.NodeAccessMethod{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.NodeAccessMethod{}, err
	}
	return method, nil
}

func normalizeNodeSSHAccessMethodCreateInput(input domain.NodeSSHAccessMethodCreateInput) (domain.NodeSSHAccessMethodCreateInput, []byte, error) {
	input.SSHHost = strings.TrimSpace(input.SSHHost)
	input.SSHUser = strings.TrimSpace(input.SSHUser)
	input.SSHHostKeySHA256 = strings.TrimSpace(input.SSHHostKeySHA256)
	if input.SSHHost == "" {
		return input, nil, domain.ValidationError{Message: "ssh_host is required"}
	}
	if !isSafeSSHAccessHost(input.SSHHost) {
		return input, nil, domain.ValidationError{Message: "ssh_host contains unsafe characters"}
	}
	if input.SSHPort == 0 {
		input.SSHPort = 22
	}
	if input.SSHPort < 1 || input.SSHPort > 65535 {
		return input, nil, domain.ValidationError{Message: "ssh_port must be between 1 and 65535"}
	}
	if input.SSHUser == "" {
		return input, nil, domain.ValidationError{Message: "ssh_user is required"}
	}
	if !sshUserPattern.MatchString(input.SSHUser) {
		return input, nil, domain.ValidationError{Message: "ssh_user contains unsafe characters"}
	}
	if input.SSHHostKeySHA256 == "" {
		return input, nil, domain.ValidationError{Message: "ssh_host_key_sha256 is required"}
	}
	if !sshHostKeySHA256Pattern.MatchString(input.SSHHostKeySHA256) {
		return input, nil, domain.ValidationError{Message: "ssh_host_key_sha256 is invalid"}
	}
	if len(input.PrivateKey) > maxNodeSSHPrivateKeyBytes {
		return input, nil, domain.ValidationError{Message: "private_key exceeds maximum size"}
	}
	if strings.TrimSpace(input.PrivateKey) == "" {
		return input, nil, domain.ValidationError{Message: "private_key is required"}
	}
	privateKey := []byte(input.PrivateKey)
	if err := validateNodeSSHPrivateKey(privateKey); err != nil {
		zeroBytes(privateKey)
		return input, nil, err
	}
	return input, privateKey, nil
}

func validateNodeSSHPrivateKey(privateKey []byte) error {
	value := strings.TrimSpace(string(privateKey))
	switch {
	case strings.HasPrefix(value, "ssh-rsa "),
		strings.HasPrefix(value, "ssh-ed25519 "),
		strings.HasPrefix(value, "ecdsa-sha2-"),
		strings.HasPrefix(value, "sk-ssh-"):
		return domain.ValidationError{Message: "private_key must contain private key material, not a public key"}
	}
	if _, err := ssh.ParseRawPrivateKey(privateKey); err != nil {
		var passphraseMissing *ssh.PassphraseMissingError
		if errors.As(err, &passphraseMissing) {
			return domain.ValidationError{Message: "private_key must be unencrypted; passphrase-protected keys are not supported"}
		}
		return domain.ValidationError{Message: "private_key is not a valid unencrypted SSH private key"}
	}
	return nil
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func insertAuditForUser(ctx context.Context, exec auditInsertExecutor, userID *string, action, resource string, resourceID *string, summary string) (domain.AuditEvent, error) {
	a := domain.AuditEvent{
		ID:           id.New(),
		ActorUserID:  userID,
		ActorType:    "platform_user",
		Action:       action,
		ResourceType: resource,
		ResourceID:   resourceID,
		Summary:      summary,
		CreatedAt:    time.Now().UTC(),
	}
	_, err := exec.Exec(ctx, `insert into audit_events(id,actor_user_id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7,'{}'::jsonb,$8)`,
		a.ID, a.ActorUserID, a.ActorType, a.Action, a.ResourceType, a.ResourceID, a.Summary, a.CreatedAt)
	return a, err
}
