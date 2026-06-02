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
	for _, required := range []string{
		"Required:",
		"--client",
		"--name",
		"Minimum valid call:",
		"wowdoc api lookup --client retail --name C_AuctionHouse.GetItemSearchResultInfo",
		"Source resolution:",
		"Agent next step:",
		"MCP arguments:",
		"client_required",
		"git_unavailable_archive_failed",
		"index_unavailable",
		"timeout",
		"unsupported_ref",
	} {
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

func TestClientsListAcceptsDiagnosticsFlag(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"clients", "list", "--include-diagnostics"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clients list diagnostics flag should execute, got %v output %s", err, out.String())
	}
}

func TestLookupAcceptsDocumentedSourceFlags(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "C_Test.Foo",
		"--source-root", "sources",
		"--source-path", "sources/retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("documented source flags should execute, got %v output %s", err, out.String())
	}
}
