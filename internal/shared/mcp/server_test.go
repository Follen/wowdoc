package mcp

import "testing"

func TestNewServerRegistersAllToolSchemas(t *testing.T) {
	server := NewServer(ServerOptions{Name: "wowdoc-test", Version: "v0.0.0-test"})

	schemas := ToolInputSchemas()
	names := server.RegisteredToolNames()
	if len(names) != 14 {
		t.Fatalf("registered tool count = %d, want 14: %#v", len(names), names)
	}
	if len(names) != len(schemas) {
		t.Fatalf("registered tool count = %d, schema count = %d", len(names), len(schemas))
	}
	if server.SDKRegisteredToolCount() != len(schemas) {
		t.Fatalf("sdk registered tool count = %d, schema count = %d", server.SDKRegisteredToolCount(), len(schemas))
	}
	for name := range schemas {
		if !server.HasTool(name) {
			t.Fatalf("server missing schema-backed tool %q", name)
		}
	}
}
