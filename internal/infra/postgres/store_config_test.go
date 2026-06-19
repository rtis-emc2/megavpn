package postgres

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestStoreArtifactRoot(t *testing.T) {
	t.Parallel()

	store := New(nil)
	if got := store.ArtifactRoot(); got != defaultArtifactRoot {
		t.Fatalf("default artifact root = %q, want %q", got, defaultArtifactRoot)
	}

	store.SetArtifactRoot(" /srv/megavpn/artifacts ")
	if got := store.ArtifactRoot(); got != "/srv/megavpn/artifacts" {
		t.Fatalf("custom artifact root = %q, want /srv/megavpn/artifacts", got)
	}

	store.SetArtifactRoot(" ")
	if got := store.ArtifactRoot(); got != "/srv/megavpn/artifacts" {
		t.Fatalf("blank SetArtifactRoot changed root to %q", got)
	}
}

func TestNodeJobLockUsesNodeIDBeforeScopeID(t *testing.T) {
	t.Parallel()

	scopeID := "backhaul-link"
	nodeID := "node-egress"
	resourceType, resourceID, lockKind, ok := jobLockTarget(domain.Job{
		Type:      "node.backhaul.apply",
		ScopeType: "backhaul",
		ScopeID:   &scopeID,
		NodeID:    &nodeID,
	})
	if !ok {
		t.Fatal("expected node job lock target")
	}
	if resourceType != "node" || resourceID != nodeID || lockKind != "bootstrap" {
		t.Fatalf("lock target = %s/%s/%s, want node/%s/bootstrap", resourceType, resourceID, lockKind, nodeID)
	}
}

func TestLongNodeJobLeaseDuration(t *testing.T) {
	t.Parallel()

	if got := jobLeaseDurationForType("node.backhaul.apply"); got != longNodeJobLeaseDuration {
		t.Fatalf("node.backhaul.apply lease = %s, want %s", got, longNodeJobLeaseDuration)
	}
	if got := jobLeaseDurationForType("node.inventory"); got != jobLeaseDuration {
		t.Fatalf("node.inventory lease = %s, want %s", got, jobLeaseDuration)
	}
}
