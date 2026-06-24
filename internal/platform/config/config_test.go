package config

import "testing"

func TestLoadProductionMode(t *testing.T) {
	t.Setenv("MEGAVPN_PRODUCTION_MODE", "true")

	cfg := Load()
	if !cfg.API.ProductionMode {
		t.Fatal("API.ProductionMode = false, want true")
	}
}
