package http

import (
	"context"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type shareLinkRotateStore struct {
	revokedClientID string
	revokedLinkID   string
	publishClientID string
	publishTargetID string
	publishTTL      time.Duration
}

func (s *shareLinkRotateStore) RevokeShareLink(_ context.Context, clientID, linkID string) (domain.ShareLink, error) {
	s.revokedClientID = clientID
	s.revokedLinkID = linkID
	return domain.ShareLink{
		ID:              linkID,
		ClientAccountID: clientID,
		TargetType:      "artifact",
		TargetID:        "artifact-1",
		Status:          "revoked",
	}, nil
}

func (s *shareLinkRotateStore) PublishShareLink(_ context.Context, clientID string, targetID string, ttl time.Duration) (domain.ShareLink, error) {
	s.publishClientID = clientID
	s.publishTargetID = targetID
	s.publishTTL = ttl
	return domain.ShareLink{
		ID:              "share-new",
		ClientAccountID: clientID,
		TargetType:      "artifact",
		TargetID:        targetID,
		Token:           "plain-once-token",
		TokenHint:       "plain-on...-token",
		Status:          "active",
		ExpiresAt:       time.Now().UTC().Add(ttl),
	}, nil
}

func TestRotateShareLinkTokenRevokesOldLinkAndPublishesSameTarget(t *testing.T) {
	t.Parallel()

	store := &shareLinkRotateStore{}
	link, err := rotateShareLinkToken(context.Background(), store, "client-1", "share-old", 12*time.Hour)
	if err != nil {
		t.Fatalf("rotate share link: %v", err)
	}
	if store.revokedClientID != "client-1" || store.revokedLinkID != "share-old" {
		t.Fatalf("revoke args = %s/%s, want client-1/share-old", store.revokedClientID, store.revokedLinkID)
	}
	if store.publishClientID != "client-1" || store.publishTargetID != "artifact-1" {
		t.Fatalf("publish args = %s/%s, want client-1/artifact-1", store.publishClientID, store.publishTargetID)
	}
	if store.publishTTL != 12*time.Hour {
		t.Fatalf("publish ttl = %s, want 12h", store.publishTTL)
	}
	if link.Token == "" || link.TokenHint == "" {
		t.Fatalf("rotated share link should return one-time token material for display: %#v", link)
	}
}
