package postgres

import "testing"

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
