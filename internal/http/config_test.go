package http

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultHTTPConfigDisablesArbitraryRefs(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Sources.AllowArbitraryRef {
		t.Fatalf("HTTP must disable arbitrary refs by default")
	}
	if cfg.Server.Port != 9789 || cfg.Limits.MaxConcurrentSourceFetches != 2 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.Prepare.PrewarmOnStart || len(cfg.Prepare.PrewarmClients) != 0 {
		t.Fatalf("prewarm should be disabled by default: %#v", cfg.Prepare)
	}
}

func TestDefaultHTTPConfigIncludesDefaultSourceSeeds(t *testing.T) {
	cfg := DefaultConfig()
	want := map[string]string{
		"retail":        "live",
		"classic":       "classic",
		"classic-ptr":   "classic_ptr",
		"classic-titan": "classic_titan",
		"ptr":           "ptr2",
		"ptr2":          "ptr2",
	}
	if len(cfg.Sources.Defaults) != len(want) {
		t.Fatalf("default source count = %d, want %d: %#v", len(cfg.Sources.Defaults), len(want), cfg.Sources.Defaults)
	}
	for _, entry := range cfg.Sources.Defaults {
		ref, ok := want[entry.Alias]
		if !ok {
			t.Fatalf("unexpected default source alias %q", entry.Alias)
		}
		if !strings.HasSuffix(entry.Repo, "wow-ui-source.git") || entry.Ref != ref {
			t.Fatalf("default source entry wrong: %#v, want ref %q", entry, ref)
		}
		delete(want, entry.Alias)
	}
	if len(want) != 0 {
		t.Fatalf("missing default source aliases: %#v", want)
	}
}

func TestLoadConfigMergesYAMLWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wowdoc.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  host: 127.0.0.1
  port: 9790
sources:
  root: /tmp/wowdoc-sources
  allow_arbitrary_ref: true
  default_ref: main
  extra:
    - alias: local-test
      path: /tmp/local-wow-source
contexts:
  max_source_contexts: 3
  max_index_contexts: 2
  pinned:
    - retail
limits:
  request_timeout_seconds: 12
prepare:
  prewarm_on_start: true
  refresh_interval_minutes: 30
  prewarm_clients:
    - retail
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 9790 {
		t.Fatalf("server config not loaded: %#v", cfg.Server)
	}
	if cfg.Sources.Root != "/tmp/wowdoc-sources" || !cfg.Sources.AllowArbitraryRef || cfg.Sources.DefaultRef != "main" {
		t.Fatalf("source config not loaded: %#v", cfg.Sources)
	}
	if len(cfg.Sources.Extra) != 1 || cfg.Sources.Extra[0].Alias != "local-test" || cfg.Sources.Extra[0].Path != "/tmp/local-wow-source" {
		t.Fatalf("extra sources not loaded: %#v", cfg.Sources.Extra)
	}
	if cfg.Contexts.MaxSourceContexts != 3 || cfg.Contexts.MaxIndexContexts != 2 || len(cfg.Contexts.Pinned) != 1 {
		t.Fatalf("context config not loaded: %#v", cfg.Contexts)
	}
	if cfg.Limits.RequestTimeoutSeconds != 12 {
		t.Fatalf("limit override not loaded: %#v", cfg.Limits)
	}
	if cfg.Limits.MaxConcurrentSourceFetches != 2 {
		t.Fatalf("default limit should be preserved: %#v", cfg.Limits)
	}
	if !cfg.Prepare.PrewarmOnStart || cfg.Prepare.RefreshIntervalMinutes != 30 || len(cfg.Prepare.PrewarmClients) != 1 || cfg.Prepare.PrewarmClients[0] != "retail" {
		t.Fatalf("prepare config not loaded: %#v", cfg.Prepare)
	}
}
