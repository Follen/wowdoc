package tools

import "testing"

func TestRegistryContainsExactlySupportedTools(t *testing.T) {
	reg := DefaultRegistry()
	want := []string{
		"list_clients",
		"lookup_blizzard_api",
		"search_blizzard_api",
		"get_api_namespace",
		"get_api_events",
		"search_framexml",
		"validate_toc",
		"check_api_deprecation",
		"suggest_api_migration",
		"get_wow_constants",
		"get_widget_api",
		"find_mixin_template",
		"lookup_cvar",
		"explain_api_safety",
	}
	if len(reg.Tools) != len(want) {
		t.Fatalf("tool count = %d, want %d", len(reg.Tools), len(want))
	}
	for _, name := range want {
		if _, ok := reg.Tools[name]; !ok {
			t.Fatalf("missing tool %q", name)
		}
	}
	if _, ok := reg.Tools["scaffold_addon"]; ok {
		t.Fatalf("omitted TypeScript tool scaffold_addon must not be registered")
	}
}
