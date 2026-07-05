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

type serviceAccessRowsCleanup struct {
	result      domain.ClientServiceAccessDeleteResult
	paths       []string
	instanceIDs []string
	nodeIDs     []string
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

func (s *Store) DeleteClientServiceAccess(ctx context.Context, clientID, accessID string) (domain.ClientServiceAccessDeleteResult, error) {
	clientID = strings.TrimSpace(clientID)
	accessID = strings.TrimSpace(accessID)
	if clientID == "" || accessID == "" {
		return domain.ClientServiceAccessDeleteResult{}, fmt.Errorf("client id and service access id are required")
	}
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.ClientServiceAccessDeleteResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ClientServiceAccessDeleteResult{}, err
	}
	defer tx.Rollback(ctx)

	cleanup, err := deleteServiceAccessRowsTx(ctx, tx, clientID, accessID)
	if err != nil {
		return domain.ClientServiceAccessDeleteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ClientServiceAccessDeleteResult{}, err
	}

	cleanup.result.Deleted = true
	cleanup.result.ConfigCleanup.FilesDeleted, cleanup.result.ConfigCleanup.FileErrors = s.removeManagedArtifactFiles(cleanup.paths)
	cleanup.result.InstanceApplyJobsQueued, cleanup.result.RoutePolicyJobsQueued, cleanup.result.QueueErrors = s.queueClientCleanupConvergence(ctx, cleanup.instanceIDs, cleanup.nodeIDs)
	_, _ = s.CreateAudit(ctx, "system", "client.service_access.delete", "client", &clientID, "client service access and generated configs deleted")
	return cleanup.result, nil
}

func (s *Store) cleanupInstanceClientServiceAccesses(ctx context.Context, tx pgx.Tx, instanceID string) (serviceAccessRowsCleanup, error) {
	out := serviceAccessRowsCleanup{}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return out, nil
	}
	rows, err := tx.Query(ctx, `select client_account_id::text,id::text from service_accesses where instance_id=$1`, instanceID)
	if err != nil {
		return out, err
	}
	type target struct {
		clientID string
		accessID string
	}
	targets := make([]target, 0)
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.clientID, &item.accessID); err != nil {
			rows.Close()
			return out, err
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	for _, item := range targets {
		cleanup, err := deleteServiceAccessRowsTx(ctx, tx, item.clientID, item.accessID)
		if err != nil {
			return out, err
		}
		out.result.ConfigCleanup.ArtifactsDeleted += cleanup.result.ConfigCleanup.ArtifactsDeleted
		out.result.ConfigCleanup.ShareLinksDeleted += cleanup.result.ConfigCleanup.ShareLinksDeleted
		out.result.ServiceAccessesDeleted += cleanup.result.ServiceAccessesDeleted
		out.result.AccessRoutesDeleted += cleanup.result.AccessRoutesDeleted
		out.result.SecretRefsDeleted += cleanup.result.SecretRefsDeleted
		out.paths = append(out.paths, cleanup.paths...)
		out.nodeIDs = append(out.nodeIDs, cleanup.nodeIDs...)
	}
	return out, nil
}

func deleteServiceAccessRowsTx(ctx context.Context, tx pgx.Tx, clientID, accessID string) (serviceAccessRowsCleanup, error) {
	clientID = strings.TrimSpace(clientID)
	accessID = strings.TrimSpace(accessID)
	out := serviceAccessRowsCleanup{
		result: domain.ClientServiceAccessDeleteResult{
			ClientID:        clientID,
			ServiceAccessID: accessID,
			ConfigCleanup:   domain.ClientConfigCleanupResult{ClientID: clientID},
		},
	}

	var instanceID string
	err := tx.QueryRow(ctx, `select instance_id::text from service_accesses where client_account_id=$1 and id=$2 for update`, clientID, accessID).Scan(&instanceID)
	if err != nil {
		return out, err
	}
	out.result.InstanceID = strings.TrimSpace(instanceID)

	activeInstanceIDs, err := activeServiceAccessInstanceIDsTx(ctx, tx, accessID)
	if err != nil {
		return out, err
	}
	out.instanceIDs = activeInstanceIDs

	nodeIDs, err := serviceAccessRouteNodeIDsTx(ctx, tx, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.nodeIDs = nodeIDs

	artifactIDs, paths, err := serviceAccessArtifactRefsTx(ctx, tx, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.paths = paths
	if len(artifactIDs) > 0 {
		tag, err := tx.Exec(ctx, `delete from share_links
			where client_account_id=$1
			  and target_type='artifact'
			  and target_id = any($2::uuid[])`, clientID, artifactIDs)
		if err != nil {
			return out, err
		}
		out.result.ConfigCleanup.ShareLinksDeleted = tag.RowsAffected()
	}

	tag, err := tx.Exec(ctx, `delete from artifacts where client_account_id=$1 and service_access_id=$2`, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.result.ConfigCleanup.ArtifactsDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from client_access_routes where client_account_id=$1 and service_access_id=$2`, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.result.AccessRoutesDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from secret_refs
		where meta_json->>'scope'='service_access'
		  and meta_json->>'service_access_id'=$1`, accessID)
	if err != nil {
		return out, err
	}
	out.result.SecretRefsDeleted = tag.RowsAffected()

	tag, err = tx.Exec(ctx, `delete from service_accesses where client_account_id=$1 and id=$2`, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.result.ServiceAccessesDeleted = tag.RowsAffected()
	if tag.RowsAffected() == 0 {
		return out, pgx.ErrNoRows
	}
	return out, nil
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

func activeServiceAccessInstanceIDsTx(ctx context.Context, tx pgx.Tx, accessID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select distinct i.id::text
		from service_accesses sa
		join instances i on i.id=sa.instance_id
		where sa.id=$1
		  and i.status not in ('deleted','deleting')`, accessID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func serviceAccessRouteNodeIDsTx(ctx context.Context, tx pgx.Tx, clientID, accessID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select distinct node_id::text
		from client_access_routes
		where client_account_id=$1
		  and service_access_id=$2
		  and node_id is not null`, clientID, accessID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func serviceAccessArtifactRefsTx(ctx context.Context, tx pgx.Tx, clientID, accessID string) ([]string, []string, error) {
	rows, err := tx.Query(ctx, `select id::text,storage_path from artifacts where client_account_id=$1 and service_access_id=$2`, clientID, accessID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	paths := make([]string, 0)
	for rows.Next() {
		var artifactID string
		var path string
		if err := rows.Scan(&artifactID, &path); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(artifactID) != "" {
			ids = append(ids, strings.TrimSpace(artifactID))
		}
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return ids, paths, nil
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
	deleted, errs := s.removeManagedArtifactFiles(paths)
	if dir, ok := s.managedClientArtifactDir(clientID); ok {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Sprintf("remove artifact directory %s: %v", dir, err))
		}
	}
	return deleted, errs
}

func (s *Store) removeManagedArtifactFiles(paths []string) (int64, []string) {
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
