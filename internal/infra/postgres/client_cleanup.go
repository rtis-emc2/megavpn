package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

type clientConfigRowsCleanup struct {
	result domain.ClientConfigCleanupResult
	paths  []string
}

func (s *Store) ClearClientConfigs(ctx context.Context, clientID string) (domain.ClientConfigCleanupResult, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return domain.ClientConfigCleanupResult{}, fmt.Errorf("client id is required")
	}
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.ClientConfigCleanupResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ClientConfigCleanupResult{}, err
	}
	defer tx.Rollback(ctx)

	cleanup, err := deleteClientConfigRowsTx(ctx, tx, clientID)
	if err != nil {
		return domain.ClientConfigCleanupResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ClientConfigCleanupResult{}, err
	}

	cleanup.result.FilesDeleted, cleanup.result.FileErrors = s.removeClientArtifactFiles(clientID, cleanup.paths)
	_, _ = s.CreateAudit(ctx, "system", "client.configs.clear", "client", &clientID, "client generated configs cleared")
	return cleanup.result, nil
}

func (s *Store) DeleteClient(ctx context.Context, clientID string) (domain.ClientDeleteResult, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return domain.ClientDeleteResult{}, fmt.Errorf("client id is required")
	}
	client, err := s.GetClient(ctx, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	defer tx.Rollback(ctx)

	instanceIDs, err := clientServiceInstanceIDsTx(ctx, tx, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	routeNodeIDs, err := clientRouteNodeIDsTx(ctx, tx, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	serviceAccessIDs, err := clientServiceAccessIDsTx(ctx, tx, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}

	configCleanup, err := deleteClientConfigRowsTx(ctx, tx, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}

	result := domain.ClientDeleteResult{
		ClientID:      clientID,
		Username:      client.Username,
		ConfigCleanup: configCleanup.result,
	}

	tag, err := tx.Exec(ctx, `delete from client_email_deliveries where client_account_id=$1`, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	result.EmailDeliveriesDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from client_access_routes where client_account_id=$1`, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	result.AccessRoutesDeleted = tag.RowsAffected()

	if len(serviceAccessIDs) > 0 {
		tag, err = tx.Exec(ctx, `delete from secret_refs
			where meta_json->>'scope'='service_access'
			  and meta_json->>'service_access_id' = any($1::text[])`, serviceAccessIDs)
		if err != nil {
			return domain.ClientDeleteResult{}, err
		}
		result.SecretRefsDeleted = tag.RowsAffected()
	}

	tag, err = tx.Exec(ctx, `delete from service_accesses where client_account_id=$1`, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	result.ServiceAccessesDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from client_accounts where id=$1`, clientID)
	if err != nil {
		return domain.ClientDeleteResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.ClientDeleteResult{}, pgx.ErrNoRows
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ClientDeleteResult{}, err
	}

	result.Deleted = true
	result.ConfigCleanup.FilesDeleted, result.ConfigCleanup.FileErrors = s.removeClientArtifactFiles(clientID, configCleanup.paths)
	result.InstanceApplyJobsQueued, result.RoutePolicyJobsQueued, result.QueueErrors = s.queueClientCleanupConvergence(ctx, instanceIDs, routeNodeIDs)
	_, _ = s.CreateAudit(ctx, "system", "client.delete", "client", &clientID, "client account and generated configs deleted")
	return result, nil
}

func deleteClientConfigRowsTx(ctx context.Context, tx pgx.Tx, clientID string) (clientConfigRowsCleanup, error) {
	out := clientConfigRowsCleanup{
		result: domain.ClientConfigCleanupResult{ClientID: clientID},
	}
	paths, err := clientArtifactPathsTx(ctx, tx, clientID)
	if err != nil {
		return out, err
	}
	out.paths = paths

	tag, err := tx.Exec(ctx, `delete from share_links where client_account_id=$1`, clientID)
	if err != nil {
		return out, err
	}
	out.result.ShareLinksDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from client_subscriptions where client_account_id=$1`, clientID)
	if err != nil {
		return out, err
	}
	out.result.SubscriptionsDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from artifacts where client_account_id=$1`, clientID)
	if err != nil {
		return out, err
	}
	out.result.ArtifactsDeleted = tag.RowsAffected()
	return out, nil
}

func clientArtifactPathsTx(ctx context.Context, tx pgx.Tx, clientID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select storage_path from artifacts where client_account_id=$1`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	return paths, rows.Err()
}

func clientServiceInstanceIDsTx(ctx context.Context, tx pgx.Tx, clientID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select distinct instance_id::text from service_accesses where client_account_id=$1 and instance_id is not null`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func clientRouteNodeIDsTx(ctx context.Context, tx pgx.Tx, clientID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select distinct node_id::text from client_access_routes where client_account_id=$1 and node_id is not null`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func clientServiceAccessIDsTx(ctx context.Context, tx pgx.Tx, clientID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select id::text from service_accesses where client_account_id=$1`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func scanStringColumn(rows pgx.Rows) ([]string, error) {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *Store) queueClientCleanupConvergence(ctx context.Context, instanceIDs, nodeIDs []string) (int64, int64, []string) {
	var instanceJobs int64
	var routeJobs int64
	var errs []string
	for _, instanceID := range instanceIDs {
		instanceID = strings.TrimSpace(instanceID)
		if instanceID == "" {
			continue
		}
		if _, err := s.UpdateInstanceStatus(ctx, instanceID, "apply"); err != nil {
			errs = append(errs, fmt.Sprintf("instance %s apply queue failed: %v", instanceID, err))
			continue
		}
		instanceJobs++
	}
	for _, nodeID := range nodeIDs {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			continue
		}
		if _, err := s.CreateNodeRoutePolicyApplyJob(ctx, nodeID); err != nil {
			errs = append(errs, fmt.Sprintf("node %s route policy queue failed: %v", nodeID, err))
			continue
		}
		routeJobs++
	}
	return instanceJobs, routeJobs, errs
}

func (s *Store) removeClientArtifactFiles(clientID string, paths []string) (int64, []string) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return 0, nil
	}
	var deleted int64
	var errs []string
	seen := map[string]struct{}{}
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if !s.artifactPathIsManaged(path) {
			errs = append(errs, fmt.Sprintf("skip unmanaged artifact path %s", path))
			continue
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Sprintf("remove %s: %v", path, err))
			continue
		}
		deleted++
	}
	if dir, ok := s.managedClientArtifactDir(clientID); ok {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Sprintf("remove artifact directory %s: %v", dir, err))
		}
	}
	return deleted, errs
}

func (s *Store) artifactPathIsManaged(path string) bool {
	root := filepath.Clean(s.ArtifactRoot())
	if strings.TrimSpace(root) == "" || strings.TrimSpace(path) == "" {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func (s *Store) managedClientArtifactDir(clientID string) (string, bool) {
	root := filepath.Clean(s.ArtifactRoot())
	clientID = strings.TrimSpace(clientID)
	if root == "" || root == "." || root == string(os.PathSeparator) || clientID == "" {
		return "", false
	}
	dir := filepath.Clean(filepath.Join(root, clientID))
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return dir, true
}
