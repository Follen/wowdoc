package mcp

import (
	"strings"
	"testing"
)

func TestSourceBackedSchemasRequireClientAndAllowOptionalRef(t *testing.T) {
	schema := ToolInputSchemas()["lookup_blizzard_api"]
	if schema.Properties["client"].Type != "string" || schema.Properties["ref"].Type != "string" {
		t.Fatalf("schema must include client/ref strings: %#v", schema)
	}
	if !schema.Requires("client") || schema.Requires("ref") {
		t.Fatalf("client required and ref optional required list wrong: %#v", schema.Required)
	}
}

func TestValidateTOCDoesNotRequireClient(t *testing.T) {
	schema := ToolInputSchemas()["validate_toc"]
	if schema.Requires("client") {
		t.Fatalf("validate_toc client must be optional")
	}
}

func TestInspectRemoteRefsSchemaAcceptsOptionalClientAndVersionFlag(t *testing.T) {
	schema := ToolInputSchemas()["inspect_remote_refs"]
	if schema.Properties["client"].Type != "string" || schema.Properties["includeVersion"].Type != "boolean" {
		t.Fatalf("inspect_remote_refs schema wrong: %#v", schema)
	}
	if schema.Requires("client") || schema.Requires("includeVersion") {
		t.Fatalf("inspect_remote_refs inputs must be optional: %#v", schema.Required)
	}
}

func TestToolSchemasExposeToolSpecificPrimaryInputs(t *testing.T) {
	schemas := ToolInputSchemas()
	cases := map[string]string{
		"lookup_blizzard_api":   "name",
		"search_blizzard_api":   "query",
		"get_api_namespace":     "namespace",
		"get_api_events":        "event",
		"search_framexml":       "query",
		"check_api_deprecation": "luaCode",
		"suggest_api_migration": "oldFunction",
		"get_wow_constants":     "name",
		"get_widget_api":        "widgetType",
		"find_mixin_template":   "name",
		"lookup_cvar":           "name",
		"explain_api_safety":    "symbol",
	}
	for tool, primary := range cases {
		schema := schemas[tool]
		if schema.Properties[primary].Type != "string" {
			t.Fatalf("%s schema missing primary string %s: %#v", tool, primary, schema)
		}
	}
}

func TestToolSchemasRequireToolSpecificPrimaryInputs(t *testing.T) {
	schemas := ToolInputSchemas()
	cases := map[string]string{
		"lookup_blizzard_api":   "name",
		"search_blizzard_api":   "query",
		"get_api_namespace":     "namespace",
		"get_api_events":        "event",
		"search_framexml":       "query",
		"check_api_deprecation": "luaCode",
		"suggest_api_migration": "oldFunction",
		"get_wow_constants":     "name",
		"get_widget_api":        "widgetType",
		"find_mixin_template":   "name",
		"lookup_cvar":           "name",
		"explain_api_safety":    "symbol",
	}
	for tool, primary := range cases {
		schema := schemas[tool]
		if !schema.Requires(primary) {
			t.Fatalf("%s schema should require primary input %s: %#v", tool, primary, schema.Required)
		}
	}
}

func TestSearchBlizzardAPISchemaExposesFilters(t *testing.T) {
	schema := ToolInputSchemas()["search_blizzard_api"]
	for _, field := range []string{"type", "limit", "safety", "scenario", "includeUnsafeOnly"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("search_blizzard_api schema missing %s: %#v", field, schema)
		}
	}
	if schema.Properties["limit"].Type != "number" {
		t.Fatalf("limit schema type = %q, want number", schema.Properties["limit"].Type)
	}
	if schema.Properties["includeUnsafeOnly"].Type != "boolean" {
		t.Fatalf("includeUnsafeOnly schema type = %q, want boolean", schema.Properties["includeUnsafeOnly"].Type)
	}
}

func TestSearchFrameXMLSchemaExposesContextOptions(t *testing.T) {
	schema := ToolInputSchemas()["search_framexml"]
	for _, field := range []string{"filePattern", "contextLines", "maxResults"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("search_framexml schema missing %s: %#v", field, schema)
		}
	}
	if schema.Properties["contextLines"].Type != "number" {
		t.Fatalf("contextLines schema type = %q, want number", schema.Properties["contextLines"].Type)
	}
}

func TestLookupCVarSchemaExposesDetail(t *testing.T) {
	schema := ToolInputSchemas()["lookup_cvar"]
	if schema.Properties["detail"].Type != "boolean" {
		t.Fatalf("lookup_cvar detail schema wrong: %#v", schema)
	}
}

func TestLookupBlizzardAPISchemaExposesExactAndIncludeSafety(t *testing.T) {
	schema := ToolInputSchemas()["lookup_blizzard_api"]
	for _, field := range []string{"exact", "includeSafety"} {
		if schema.Properties[field].Type != "boolean" {
			t.Fatalf("lookup_blizzard_api %s schema wrong: %#v", field, schema)
		}
	}
	if !strings.Contains(schema.Properties["exact"].Description, "Defaults to false") || !strings.Contains(schema.Properties["exact"].Description, "fuzzy") {
		t.Fatalf("lookup_blizzard_api exact schema should document fuzzy default: %#v", schema.Properties["exact"])
	}
}

func TestGetWowConstantsSchemaExposesKindAndLimit(t *testing.T) {
	schema := ToolInputSchemas()["get_wow_constants"]
	if schema.Properties["kind"].Type != "string" || schema.Properties["limit"].Type != "number" || schema.Properties["filter"].Type != "string" {
		t.Fatalf("get_wow_constants schema missing filter/kind/limit: %#v", schema)
	}
}

func TestGetAPIEventsSchemaExposesFilter(t *testing.T) {
	schema := ToolInputSchemas()["get_api_events"]
	if schema.Properties["filter"].Type != "string" {
		t.Fatalf("get_api_events filter schema wrong: %#v", schema)
	}
}

func TestFindMixinTemplateSchemaExposesKindAndLimit(t *testing.T) {
	schema := ToolInputSchemas()["find_mixin_template"]
	if schema.Properties["kind"].Type != "string" || schema.Properties["limit"].Type != "number" {
		t.Fatalf("find_mixin_template schema missing kind/limit: %#v", schema)
	}
}
