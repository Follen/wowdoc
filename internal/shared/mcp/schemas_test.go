package mcp

import "testing"

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
