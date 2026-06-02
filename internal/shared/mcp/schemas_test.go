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
