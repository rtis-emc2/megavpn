package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) ResolveShareLinkArtifact(ctx context.Context, token string) (domain.ShareLink, domain.Artifact, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.ShareLink{}, domain.Artifact{}, errors.New("share token is required")
	}
	var link domain.ShareLink
	err := s.db.QueryRow(ctx, `select id,client_account_id,target_type,target_id,coalesce(token_hint,''),status,expires_at,download_count,created_at from share_links where token_hash=$1`, hashToken(token)).
		Scan(&link.ID, &link.ClientAccountID, &link.TargetType, &link.TargetID, &link.TokenHint, &link.Status, &link.ExpiresAt, &link.DownloadCount, &link.CreatedAt)
	if err != nil {
		return domain.ShareLink{}, domain.Artifact{}, err
	}
	now := time.Now().UTC()
	if link.Status != "active" {
		return domain.ShareLink{}, domain.Artifact{}, errors.New("share link is not active")
	}
	if link.ExpiresAt.Before(now) {
		_, _ = s.db.Exec(ctx, `update share_links set status='expired' where id=$1 and status='active'`, link.ID)
		return domain.ShareLink{}, domain.Artifact{}, errors.New("share link has expired")
	}
	if link.TargetType != "artifact" {
		return domain.ShareLink{}, domain.Artifact{}, errors.New("share link target is not an artifact")
	}
	var artifact domain.Artifact
	err = s.db.QueryRow(ctx, `select id,client_account_id,service_access_id,artifact_type,storage_path,coalesce(content_hash,''),coalesce(size_bytes,0),status,created_at from artifacts where id=$1 and client_account_id=$2 and status='ready'`, link.TargetID, link.ClientAccountID).
		Scan(&artifact.ID, &artifact.ClientAccountID, &artifact.ServiceAccessID, &artifact.ArtifactType, &artifact.StoragePath, &artifact.ContentHash, &artifact.SizeBytes, &artifact.Status, &artifact.CreatedAt)
	if err != nil {
		return domain.ShareLink{}, domain.Artifact{}, err
	}
	_, _ = s.db.Exec(ctx, `update share_links set download_count=download_count+1 where id=$1`, link.ID)
	link.DownloadCount++
	return link, artifact, nil
}
