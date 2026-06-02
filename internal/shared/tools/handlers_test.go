package tools

import (
	"path/filepath"
	"testing"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
)

func TestListClientsIncludesValidClientsAndInvalidDiagnostics(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repos := []analyze.Repository{
		analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail"),
		analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic"),
		analyze.DetectRepository(filepath.Join(root, "invalid-random"), "random"),
	}

	env := ListClients(repos, true)
	if !env.OK {
		t.Fatalf("expected ok envelope: %#v", env)
	}
	if len(env.Data.Clients) != 2 {
		t.Fatalf("valid clients = %d, want 2: %#v", len(env.Data.Clients), env.Data.Clients)
	}
	if env.Data.Clients[0].Alias != "retail" || env.Data.Clients[1].Alias != "classic" {
		t.Fatalf("valid clients not preserved in order: %#v", env.Data.Clients)
	}
	if len(env.Diagnostics) != 1 || env.Diagnostics[0].Message != "source_invalid" {
		t.Fatalf("expected invalid source diagnostic: %#v", env.Diagnostics)
	}
}

func TestLookupBlizzardAPIReturnsCapabilityUnavailableForPartialClassic(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	env := LookupBlizzardAPI(repo, nil, "C_AuctionHouse.GetItemSearchResultInfo")
	if env.OK || env.Error == nil {
		t.Fatalf("expected error envelope: %#v", env)
	}
	if env.Error.Code != contracts.ErrCapabilityUnavailable {
		t.Fatalf("error code = %q, want %q", env.Error.Code, contracts.ErrCapabilityUnavailable)
	}
	if env.Source.Client != "classic" || env.Source.Version == "" {
		t.Fatalf("expected source transparency for partial classic: %#v", env.Source)
	}
}
