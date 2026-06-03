package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
	"wowdoc/internal/shared/tools"
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
		"ptr|ptr2",
		"--name",
		"Minimum valid call:",
		"wowdoc api lookup --client retail --name C_AuctionHouse.GetItemSearchResultInfo",
		"Source resolution:",
		"Agent next step:",
		"MCP arguments:",
		"default false uses fuzzy substring lookup",
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

func TestLeafCommandHelpIsAgentFriendly(t *testing.T) {
	cases := []struct {
		args     []string
		required []string
	}{
		{
			args: []string{"clients", "list", "--help"},
			required: []string{
				"First diagnostic command:",
				"Minimum valid call:",
				"wowdoc clients list --include-diagnostics",
				"Agent next step:",
				"include-diagnostics",
			},
		},
		{
			args: []string{"api", "search", "--help"},
			required: []string{
				"Required:",
				"--client",
				"--query",
				"Source resolution:",
				"Minimum valid call:",
				"wowdoc api search --client retail --query Auction",
				"MCP arguments:",
				`"query":"Auction"`,
				"Common error codes:",
			},
		},
		{
			args:     []string{"api", "events", "--help"},
			required: []string{"Required:", "--event", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"api", "namespace", "--help"},
			required: []string{"Required:", "--namespace", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"api", "deprecation", "--help"},
			required: []string{"Required:", "--lua-code", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"api", "migration", "--help"},
			required: []string{"Required:", "--old-function", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"api", "safety", "--help"},
			required: []string{"Required:", "--symbol", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"framexml", "search", "--help"},
			required: []string{"Required:", "--query", "Source resolution:", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"widget", "api", "--help"},
			required: []string{"Required:", "--widget-type", "Source resolution:", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"cvar", "lookup", "--help"},
			required: []string{"Required:", "--name", "Source resolution:", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"constants", "get", "--help"},
			required: []string{"Required:", "--name", "Source resolution:", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args:     []string{"mixin", "find", "--help"},
			required: []string{"Required:", "--name", "Source resolution:", "Minimum valid call:", "MCP arguments:", "Common error codes:"},
		},
		{
			args: []string{"toc", "validate", "--help"},
			required: []string{
				"Required:",
				"--toc-content or --toc-path",
				"Minimum valid call:",
				"wowdoc toc validate --toc-path .\\MyAddon.toc",
				"MCP arguments:",
				"Source resolution:",
			},
		},
		{
			args: []string{"mcp", "stdio", "--help"},
			required: []string{
				"Minimum valid call:",
				"wowdoc mcp stdio",
				"Source resolution:",
				"Agent next step:",
			},
		},
	}
	for _, tc := range cases {
		cmd := NewRootCommand()
		cmd.SetArgs(tc.args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v help failed: %v", tc.args, err)
		}
		text := out.String()
		for _, required := range tc.required {
			if !strings.Contains(text, required) {
				t.Fatalf("%v help missing %q:\n%s", tc.args, required, text)
			}
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

func TestAPIDeprecationRequiresLuaCode(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"api", "deprecation", "--client", "retail"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "lua_code_required") {
		t.Fatalf("expected lua_code_required, got %v output %s", err, out.String())
	}
}

func TestDefaultStdioOptionsIncludeDefaultSeeds(t *testing.T) {
	opts := defaultStdioOptions("sources")
	if opts.SourceRoot != "sources" {
		t.Fatalf("source root = %q", opts.SourceRoot)
	}
	if opts.SourceRepos["retail"] == "" || opts.DefaultRefs["retail"] == "" || opts.Git == nil || opts.Archive == nil {
		t.Fatalf("stdio options missing default source acquisition settings: %#v", opts)
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

func TestClientsListPrintsClientsAndDiagnostics(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"clients", "list", "--source-root", "../../testdata/sources", "--include-diagnostics"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clients list should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.ListClientsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("clients list output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Clients) < 2 || len(env.Diagnostics) != 1 {
		t.Fatalf("clients list envelope wrong: %#v", env)
	}
}

func TestClientsListCanIncludeRefs(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"clients", "list", "--source-root", "../../testdata/sources", "--include-refs"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clients list should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.ListClientsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("clients list output is not json: %v\n%s", err, out.String())
	}
	for _, client := range env.Data.Clients {
		if client.Alias == "valid-retail" {
			if client.DefaultRef != "latest" {
				t.Fatalf("include-refs should expose default ref: %#v", client)
			}
			return
		}
	}
	t.Fatalf("valid-retail missing from clients list: %#v", env.Data.Clients)
}

func TestSourcesRefsCommandReportsRemoteBranchMapping(t *testing.T) {
	oldGit := cliSourceGit
	cliSourceGit = remoteRefsCLIGit{
		"ptr2": "96443d533fd09a5b6195637f95c896439c616cee",
	}
	t.Cleanup(func() {
		cliSourceGit = oldGit
	})

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"sources", "refs", "--client", "ptr"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sources refs should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.RemoteRefsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("sources refs output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Clients) != 1 {
		t.Fatalf("sources refs envelope wrong: %#v", env)
	}
	got := env.Data.Clients[0]
	if got.Alias != "ptr" || got.ConfiguredRef != "ptr2" || got.Commit != "96443d533fd09a5b6195637f95c896439c616cee" {
		t.Fatalf("ptr remote ref wrong: %#v", got)
	}
}

func TestSourcesRefsCommandReportsUnknownClient(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"sources", "refs", "--client", "not-real"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sources refs should print an error envelope, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.RemoteRefsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("sources refs output is not json: %v\n%s", err, out.String())
	}
	if env.OK || env.Error == nil || env.Error.Code != contracts.ErrClientNotFound {
		t.Fatalf("sources refs unknown client envelope wrong: %#v", env)
	}
}

func TestClientsListIgnoresInternalSourceCacheDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"repos", "checkouts", "archives"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create internal cache dir: %v", err)
		}
	}
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"clients", "list", "--source-root", root, "--include-diagnostics"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clients list should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.ListClientsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("clients list output is not json: %v\n%s", err, out.String())
	}
	if len(env.Diagnostics) != 0 {
		t.Fatalf("internal cache dirs should not be diagnostics: %#v", env.Diagnostics)
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

func TestLookupWithSourcePathPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "C_AuctionHouse.GetItemSearchResultInfo",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Data.Name != "C_AuctionHouse.GetItemSearchResultInfo" || env.Source.Client != "retail" {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
	if env.Data.Namespace != "C_AuctionHouse" || env.Data.System != "C_AuctionHouse" {
		t.Fatalf("lookup namespace/system missing: %#v", env.Data)
	}
	if env.Data.Signature != "C_AuctionHouse.GetItemSearchResultInfo(itemKey, sorts) -> itemSearchResultInfo" || len(env.Data.Arguments) != 2 || len(env.Data.Returns) != 1 {
		t.Fatalf("lookup signature detail missing: %#v", env.Data)
	}
}

func TestLookupSupportsExactAndIncludeSafetyFlags(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "GetItemSearchResultInfo",
		"--include-safety=false",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Data.Name != "C_AuctionHouse.GetItemSearchResultInfo" {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
	if env.Data.Safety != nil {
		t.Fatalf("include-safety=false should omit safety: %#v", env.Data.Safety)
	}
}

func TestLookupExactTrueRequiresExactName(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "GetItemSearchResultInfo",
		"--exact=true",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Data.Name != "GetItemSearchResultInfo" || env.Data.Type != "" {
		t.Fatalf("exact=true should not fuzzy-match partial names: %#v", env)
	}
}

func TestLookupWithSourceRootUsesDefaultSeedCheckout(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkouts", "retail", "live")
	copyDir(t, filepath.Join("..", "..", "testdata", "sources", "valid-retail"), checkout)
	oldGit := cliSourceGit
	oldArchive := cliSourceArchive
	cliSourceGit = nil
	cliSourceArchive = nil
	t.Cleanup(func() {
		cliSourceGit = oldGit
		cliSourceArchive = oldArchive
	})
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "C_AuctionHouse",
		"--source-root", root,
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Path != checkout || env.Source.RequestedRef != "live" || env.Data.Name != "C_AuctionHouse" {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
}

func TestLookupWithSourceRootAcquiresDefaultSeedCheckoutWithGit(t *testing.T) {
	root := t.TempDir()
	oldGit := cliSourceGit
	oldArchive := cliSourceArchive
	cliSourceGit = &lookupFixtureGit{fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail")}
	cliSourceArchive = nil
	t.Cleanup(func() {
		cliSourceGit = oldGit
		cliSourceArchive = oldArchive
	})
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "C_AuctionHouse",
		"--source-root", root,
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Path != filepath.Join(root, "checkouts", "retail", "live") {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
}

func TestLookupWithSourceRootSupportsSlashBranchRef(t *testing.T) {
	root := t.TempDir()
	oldGit := cliSourceGit
	oldArchive := cliSourceArchive
	cliSourceGit = &lookupFixtureGit{fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail"), resolvedCommit: "abc123def456"}
	cliSourceArchive = nil
	t.Cleanup(func() {
		cliSourceGit = oldGit
		cliSourceArchive = oldArchive
	})
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--ref", "feature/auction-fix",
		"--name", "C_AuctionHouse",
		"--source-root", root,
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.RequestedRef != "feature/auction-fix" || env.Source.ResolvedRef != "abc123def456" || env.Source.Path != filepath.Join(root, "checkouts", "retail", "abc123def456") {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
}

func TestLookupWithSourceRootReportsArchiveFallbackDiagnostics(t *testing.T) {
	root := t.TempDir()
	oldGit := cliSourceGit
	oldArchive := cliSourceArchive
	cliSourceGit = failingCLIGit{}
	cliSourceArchive = &lookupFixtureArchive{fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail")}
	t.Cleanup(func() {
		cliSourceGit = oldGit
		cliSourceArchive = oldArchive
	})
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "retail",
		"--name", "C_AuctionHouse",
		"--source-root", root,
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Path != filepath.Join(root, "archives", "retail", "live") {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
	if !containsDiagnostic(env.Diagnostics, "archive fallback is not incremental") ||
		!containsDiagnostic(env.Diagnostics, "branch archives may need periodic redownload") ||
		!containsDiagnostic(env.Diagnostics, "resolved commit may be unknown for archive fallback") {
		t.Fatalf("archive fallback limitations missing from diagnostics: %#v", env.Diagnostics)
	}
}

func TestLookupWithPartialSourcePrintsCapabilityUnavailable(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "lookup",
		"--client", "classic",
		"--name", "C_AuctionHouse.GetItemSearchResultInfo",
		"--source-path", "../../testdata/sources/partial-classic",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("capability errors should be encoded, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APIResult]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("lookup output is not json: %v\n%s", err, out.String())
	}
	if env.OK || env.Error == nil || env.Error.Code != contracts.ErrCapabilityUnavailable {
		t.Fatalf("expected capability_unavailable envelope: %#v", env)
	}
}

func TestAPISafetyCommandPrintsScenarioEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "safety",
		"--client", "retail",
		"--symbol", "C_AuctionHouse.GetItemSearchResultInfo",
		"--scenario", "combat",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api safety should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.SafetyExplanationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api safety output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || env.Data.Explanation.Scenario != "combat" {
		t.Fatalf("api safety envelope wrong: %#v", env)
	}
}

func TestAPISafetyCommandIncludesRawMetadataAndClassification(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "safety",
		"--client", "retail",
		"--symbol", "C_AuctionHouse.GetRestrictedInfo",
		"--scenario", "unit_cast",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api safety should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.SafetyExplanationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api safety output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Data.Raw.SecretWrapperConstant != "ContextuallySecret" || env.Data.Classification.Level != analyze.RiskConditionalSecret {
		t.Fatalf("api safety should include raw metadata and classification: %#v", env)
	}
}

func TestAPISearchCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "search",
		"--client", "retail",
		"--query", "Auction",
		"--type", "Event",
		"--limit", "1",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api search should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APISearchData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api search output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Results) != 1 || env.Data.Results[0].Type != "Event" {
		t.Fatalf("api search envelope wrong: %#v", env)
	}
}

func TestAPISearchCommandSupportsUnsafeOnlyScenarioFilters(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "search",
		"--client", "retail",
		"--query", "Auction",
		"--include-unsafe-only",
		"--scenario", "combat",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api search should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APISearchData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api search output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Results) != 2 {
		t.Fatalf("unsafe-only api search envelope wrong: %#v", env)
	}
	for _, result := range env.Data.Results {
		if result.Safety == nil || result.Safety.Classification.Level == analyze.RiskSafe || result.Safety.Classification.Level == analyze.RiskNeverSecret {
			t.Fatalf("unsafe-only api search returned safe result: %#v", result)
		}
	}
}

func TestAPIEventsCommandPrintsPayloadEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "events",
		"--client", "retail",
		"--event", "AUCTION_HOUSE_SHOW",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api events should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.EventData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api events output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Results) == 0 || len(env.Data.Results[0].Arguments) == 0 {
		t.Fatalf("api events envelope wrong: %#v", env)
	}
}

func TestAPIEventsCommandSupportsPayloadFilter(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "events",
		"--client", "retail",
		"--event", "list",
		"--filter", "isAethereal",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api events should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.EventData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api events output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "AUCTION_HOUSE_SHOW" {
		t.Fatalf("api events filter envelope wrong: %#v", env)
	}
}

func TestAPINamespaceCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "namespace",
		"--client", "retail",
		"--namespace", "C_AuctionHouse",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api namespace should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APISearchData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api namespace output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Results) == 0 {
		t.Fatalf("api namespace envelope wrong: %#v", env)
	}
}

func TestAPINamespaceListCommandReturnsOnlyNamespaces(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "namespace",
		"--client", "retail",
		"--namespace", "list",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api namespace list should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.APISearchData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api namespace output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "C_AuctionHouse" || env.Data.Results[0].Type != "System" {
		t.Fatalf("namespace=list should only return namespaces: %#v", env)
	}
}

func TestAPIDeprecationCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "deprecation",
		"--client", "retail",
		"--lua-code", "local item = GetItemInfo(19019)",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api deprecation should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.DeprecationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api deprecation output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Deprecated) == 0 {
		t.Fatalf("api deprecation envelope wrong: %#v", env)
	}
}

func TestAPIDeprecationCommandWarnsForClassicLikeClients(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "deprecation",
		"--client", "partial-classic",
		"--lua-code", "GetContainerItemInfo(0, 1)",
		"--source-root", "../../testdata/sources",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api deprecation should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.DeprecationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api deprecation output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || !containsText(env.Data.Warnings, "Classic") {
		t.Fatalf("classic-like deprecation warning missing: %#v", env)
	}
}

func TestAPIMigrationCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "migration",
		"--client", "retail",
		"--old-function", "GetContainerItemInfo",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api migration should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.MigrationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api migration output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Suggestions) == 0 {
		t.Fatalf("api migration envelope wrong: %#v", env)
	}
}

func TestAPIMigrationCommandWarnsForClassicLikeClients(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"api", "migration",
		"--client", "partial-classic",
		"--old-function", "GetContainerItemInfo",
		"--source-root", "../../testdata/sources",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("api migration should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.MigrationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("api migration output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || !containsText(env.Data.Warnings, "Classic") {
		t.Fatalf("classic-like migration warning missing: %#v", env)
	}
}

func TestFrameXMLSearchCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"framexml", "search",
		"--client", "classic",
		"--query", "SecureActionButtonTemplate",
		"--file-pattern", "SecureTemplates.lua",
		"--context-lines", "1",
		"--source-path", "../../testdata/sources/partial-classic",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("framexml search should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.FrameXMLSearchData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("framexml search output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "classic" || len(env.Data.Results) == 0 || len(env.Data.Results[0].Before) != 1 || len(env.Data.Results[0].After) != 1 {
		t.Fatalf("framexml search envelope wrong: %#v", env)
	}
}

func TestWidgetAPICommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"widget", "api",
		"--client", "retail",
		"--widget-type", "Button",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("widget api should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.WidgetAPIData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("widget api output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || env.Data.WidgetType != "Button" || len(env.Data.Results) == 0 {
		t.Fatalf("widget api envelope wrong: %#v", env)
	}
}

func TestConstantsGetCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"constants", "get",
		"--client", "retail",
		"--name", "Enum.ItemQuality",
		"--kind", "Enumeration",
		"--limit", "1",
		"--source-path", "../../testdata/sources/valid-retail-constants",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("constants get should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.ConstantsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("constants get output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Results) == 0 {
		t.Fatalf("constants get envelope wrong: %#v", env)
	}
	if len(env.Data.Results[0].Fields) == 0 || len(env.Data.Results[0].Values) == 0 {
		t.Fatalf("constants get should include fields and values: %#v", env.Data.Results[0])
	}
}

func TestConstantsGetCommandSupportsFilter(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"constants", "get",
		"--client", "retail",
		"--name", "list",
		"--filter", "Power",
		"--source-path", "../../testdata/sources/valid-retail-constants",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("constants get should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.ConstantsData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("constants get output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "Enum.PowerType" {
		t.Fatalf("constants filter envelope wrong: %#v", env)
	}
}

func TestMixinFindCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"mixin", "find",
		"--client", "classic",
		"--name", "SecureActionButtonTemplate",
		"--source-path", "../../testdata/sources/partial-classic",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mixin find should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.MixinTemplateData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("mixin find output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "classic" || len(env.Data.Results) == 0 {
		t.Fatalf("mixin find envelope wrong: %#v", env)
	}
}

func TestMixinFindCommandSupportsKindFilter(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"mixin", "find",
		"--client", "classic",
		"--name", "SecureActionButton",
		"--kind", "mixin",
		"--source-path", "../../testdata/sources/partial-classic",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mixin find should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.MixinTemplateData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("mixin find output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "SecureActionButtonMixin" || env.Data.Results[0].Kind != "Mixin" {
		t.Fatalf("mixin kind filter envelope wrong: %#v", env)
	}
}

func TestCVarLookupCommandPrintsJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"cvar", "lookup",
		"--client", "retail",
		"--name", "graphicsQuality",
		"--detail",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cvar lookup should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.CVarData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("cvar lookup output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Source.Client != "retail" || len(env.Data.Results) == 0 || env.Data.Results[0].Name != "graphicsQuality" || env.Data.Results[0].DefaultValue != "5" {
		t.Fatalf("cvar lookup envelope wrong: %#v", env)
	}
}

func TestCVarLookupCommandReturnsEmptyArrayForMissingName(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"cvar", "lookup",
		"--client", "retail",
		"--name", "definitely_missing_cvar",
		"--source-path", "../../testdata/sources/valid-retail",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cvar lookup should execute, got %v output %s", err, out.String())
	}
	if strings.Contains(out.String(), `"results":null`) || !strings.Contains(out.String(), `"results":[]`) {
		t.Fatalf("missing cvar should return results array: %s", out.String())
	}
}

func TestTOCValidateCommandCanRunWithoutClient(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"toc", "validate",
		"--toc-content", "## Title: My Addon\nmain.lua",
		"--addon-name", "My Addon",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("toc validate should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.TOCValidationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("toc validate output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || env.Data.AddonName != "My Addon" || len(env.Data.Errors) == 0 {
		t.Fatalf("toc validate envelope wrong: %#v", env)
	}
}

func TestTOCValidateCommandReadsTOCPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MyAddon.toc")
	if err := os.WriteFile(path, []byte("## Title: My Addon\nmain.lua\n"), 0o600); err != nil {
		t.Fatalf("write toc fixture: %v", err)
	}
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"toc", "validate", "--toc-path", path})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("toc validate should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.TOCValidationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("toc validate output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || len(env.Data.Errors) != 1 || !containsText(env.Data.Errors, "Missing required field: ## Interface") {
		t.Fatalf("toc validate path envelope wrong: %#v", env)
	}
}

func TestTOCValidateCommandCanUseSourceVersion(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"toc", "validate",
		"--client", "valid-retail",
		"--source-root", "../../testdata/sources",
		"--toc-content", "## Interface: 11507\n## Title: My Addon\n",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("toc validate should execute, got %v output %s", err, out.String())
	}
	var env contracts.Envelope[tools.TOCValidationData]
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("toc validate output is not json: %v\n%s", err, out.String())
	}
	if !env.OK || !containsText(env.Data.Warnings, "12.0.0.60000") {
		t.Fatalf("source-aware toc warning missing: %#v", env)
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	}); err != nil {
		t.Fatalf("copy fixture dir: %v", err)
	}
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func containsDiagnostic(values []contracts.Diagnostic, needle string) bool {
	for _, value := range values {
		if strings.Contains(value.Message, needle) {
			return true
		}
	}
	return false
}

type lookupFixtureGit struct {
	fixture        string
	resolvedCommit string
}

type failingCLIGit struct{}

func (failingCLIGit) Run(args ...string) error { return os.ErrNotExist }
func (failingCLIGit) Output(args ...string) ([]byte, error) {
	return nil, os.ErrNotExist
}

type lookupFixtureArchive struct {
	fixture string
}

func (a *lookupFixtureArchive) FetchArchive(repoURL, ref, destination string) error {
	return copyDirErr(a.fixture, destination)
}

func (g *lookupFixtureGit) Run(args ...string) error {
	if len(args) == 4 && args[0] == "clone" && args[1] == "--mirror" {
		return os.MkdirAll(args[3], 0o755)
	}
	if len(args) == 7 && args[2] == "worktree" && args[3] == "add" {
		return copyDirErr(g.fixture, args[5])
	}
	return nil
}

func (g *lookupFixtureGit) Output(args ...string) ([]byte, error) {
	if g.resolvedCommit != "" {
		return []byte(g.resolvedCommit + "\n"), nil
	}
	return []byte("live\n"), nil
}

type remoteRefsCLIGit map[string]string

func (g remoteRefsCLIGit) Run(args ...string) error { return nil }

func (g remoteRefsCLIGit) Output(args ...string) ([]byte, error) {
	if len(args) >= 4 && args[0] == "ls-remote" && args[1] == "--heads" {
		ref := args[3]
		commit := g[ref]
		if commit == "" {
			return []byte{}, nil
		}
		return []byte(commit + "\trefs/heads/" + ref + "\n"), nil
	}
	return nil, os.ErrNotExist
}

func copyDirErr(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
