package http

import "testing"

func TestDefaultHTTPConfigDisablesArbitraryRefs(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Sources.AllowArbitraryRef {
		t.Fatalf("HTTP must disable arbitrary refs by default")
	}
	if cfg.Server.Port != 9789 || cfg.Limits.MaxConcurrentSourceFetches != 2 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}
