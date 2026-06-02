package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLookupHelpIsAgentFriendly(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"api", "lookup", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	text := out.String()
	for _, required := range []string{"Required:", "--client", "--name", "Source resolution:", "Agent next step:", "MCP arguments:", "client_required"} {
		if !strings.Contains(text, required) {
			t.Fatalf("help missing %q:\n%s", required, text)
		}
	}
}

func TestLookupRequiresClient(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"api", "lookup", "--name", "C_Test.Foo"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "client_required") {
		t.Fatalf("expected client_required, got %v output %s", err, out.String())
	}
}
