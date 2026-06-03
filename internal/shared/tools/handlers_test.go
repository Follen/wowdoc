package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	env := ListClients(repos, ListClientsOptions{IncludeDiagnostics: true})
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

func TestListClientsCanIncludeRefTransparency(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	retail.RequestedRef = "main"
	retail.ResolvedRef = "abc123"

	env := ListClients([]analyze.Repository{retail}, ListClientsOptions{IncludeRefs: true})
	if !env.OK || len(env.Data.Clients) != 1 {
		t.Fatalf("expected one client: %#v", env)
	}
	client := env.Data.Clients[0]
	if client.DefaultRef != "main" || client.RequestedRef != "main" || client.ResolvedRef != "abc123" {
		t.Fatalf("ref transparency missing from client summary: %#v", client)
	}
}

func TestInspectRemoteRefsReportsConfiguredBranches(t *testing.T) {
	env := InspectRemoteRefs(map[string]string{
		"ptr":    "https://example.test/wow-ui-source.git",
		"retail": "https://example.test/wow-ui-source.git",
	}, map[string]string{
		"ptr":    "ptr2",
		"retail": "live",
	}, remoteRefsFixtureGit{
		"live": "9746fb49dbaaf708dbc1110180607154c10b55b7",
		"ptr2": "96443d533fd09a5b6195637f95c896439c616cee",
	}, nil)

	if !env.OK || len(env.Data.Clients) != 2 {
		t.Fatalf("remote refs envelope wrong: %#v", env)
	}
	if env.Data.Clients[0].Alias != "ptr" || env.Data.Clients[0].ConfiguredRef != "ptr2" || env.Data.Clients[0].Commit != "96443d533fd09a5b6195637f95c896439c616cee" || env.Data.Clients[0].Status != "ok" {
		t.Fatalf("ptr remote ref wrong: %#v", env.Data.Clients[0])
	}
	if env.Data.Clients[1].Alias != "retail" || env.Data.Clients[1].ConfiguredRef != "live" || env.Data.Clients[1].Commit != "9746fb49dbaaf708dbc1110180607154c10b55b7" || env.Data.Clients[1].Status != "ok" {
		t.Fatalf("retail remote ref wrong: %#v", env.Data.Clients[1])
	}
}

func TestInspectRemoteRefsFallsBackToSourceResolverWhenGitUnavailable(t *testing.T) {
	env := InspectRemoteRefs(map[string]string{
		"ptr": "https://example.test/wow-ui-source.git",
	}, map[string]string{
		"ptr": "ptr2",
	}, failingRemoteRefsGit{}, func(alias string) (string, string, error) {
		if alias != "ptr" {
			t.Fatalf("fallback alias = %q, want ptr", alias)
		}
		return "12.0.7.67808", "sources/archives/ptr/ptr2", nil
	})

	if !env.OK || len(env.Data.Clients) != 1 {
		t.Fatalf("remote refs envelope wrong: %#v", env)
	}
	got := env.Data.Clients[0]
	if got.Status != "fallback" || got.ConfiguredRef != "ptr2" || got.Version != "12.0.7.67808" || got.Path != "sources/archives/ptr/ptr2" {
		t.Fatalf("fallback remote ref wrong: %#v", got)
	}
}

func TestListClientsJSONUsesEmptyArrayForNoClients(t *testing.T) {
	env := ListClients(nil, ListClientsOptions{})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), `"clients":null`) || !strings.Contains(string(data), `"clients":[]`) {
		t.Fatalf("empty clients should marshal as an array: %s", string(data))
	}
}

func TestLookupBlizzardAPIReturnsCapabilityUnavailableForPartialClassic(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	env := LookupBlizzardAPI(repo, nil, LookupAPIOptions{Name: "C_AuctionHouse.GetItemSearchResultInfo", Exact: true, IncludeSafety: true})
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

func TestCapabilityUnavailableJSONDoesNotUseNullResultArray(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	env := SearchBlizzardAPI(repo, nil, APISearchOptions{Query: "Button"})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), `"results":null`) {
		t.Fatalf("capability error should not expose null result arrays: %s", string(data))
	}
}

func TestLookupBlizzardAPIReturnsSignatureArgumentsAndReturns(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "C_AuctionHouse.GetItemSearchResultInfo", Exact: true, IncludeSafety: true})
	if !env.OK {
		t.Fatalf("lookup envelope should be ok: %#v", env)
	}
	if env.Data.Signature != "C_AuctionHouse.GetItemSearchResultInfo(itemKey, sorts) -> itemSearchResultInfo" {
		t.Fatalf("signature = %q", env.Data.Signature)
	}
	if len(env.Data.Arguments) != 2 || env.Data.Arguments[0].Name != "itemKey" {
		t.Fatalf("arguments not returned: %#v", env.Data.Arguments)
	}
	if len(env.Data.Returns) != 1 || env.Data.Returns[0].Name != "itemSearchResultInfo" {
		t.Fatalf("returns not returned: %#v", env.Data.Returns)
	}
	if env.Data.Safety == nil || !env.Data.Safety.Raw.IsProtectedFunction || env.Data.Safety.Classification.Level != analyze.RiskProtected {
		t.Fatalf("safety not returned: %#v", env.Data.Safety)
	}
}

func TestLookupBlizzardAPIReturnsNamespaceAndSystem(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "C_AuctionHouse.GetItemSearchResultInfo", Exact: true, IncludeSafety: true})
	if !env.OK {
		t.Fatalf("lookup envelope should be ok: %#v", env)
	}
	if env.Data.Namespace != "C_AuctionHouse" || env.Data.System != "C_AuctionHouse" {
		t.Fatalf("namespace/system missing from lookup result: %#v", env.Data)
	}
}

func TestLookupBlizzardAPIHonorsExactAndIncludeSafety(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	partial := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "GetItemSearchResultInfo", Exact: false, IncludeSafety: true})
	if !partial.OK || partial.Data.Name != "C_AuctionHouse.GetItemSearchResultInfo" {
		t.Fatalf("exact=false lookup envelope wrong: %#v", partial)
	}

	withoutSafety := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "C_AuctionHouse.GetItemSearchResultInfo", IncludeSafety: false})
	if !withoutSafety.OK || withoutSafety.Data.Safety != nil {
		t.Fatalf("includeSafety=false should omit safety: %#v", withoutSafety.Data.Safety)
	}
}

func TestLookupBlizzardAPIJSONOmitsSafetyWhenDisabled(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "C_AuctionHouse.GetItemSearchResultInfo", IncludeSafety: false})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal lookup envelope: %v", err)
	}
	if strings.Contains(string(data), `"safety"`) {
		t.Fatalf("includeSafety=false should omit safety from JSON: %s", string(data))
	}
}

func TestLookupBlizzardAPIJSONUsesEmptyArraysForSafetySlices(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "ItemSearchResultInfo", IncludeSafety: true})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal lookup envelope: %v", err)
	}
	for _, unexpected := range []string{`"SecretArgumentsAddAspect":null`, `"SecretReturnsForAspect":null`, `"RestrictedTypes":null`, `"Fields":null`} {
		if strings.Contains(string(data), unexpected) {
			t.Fatalf("safety slices should marshal as empty arrays, found %s in %s", unexpected, string(data))
		}
	}
}

func TestLookupBlizzardAPIJSONUsesEmptyArraysForNoReturnFunctions(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "C_AuctionHouse.StartCommoditiesPurchase", IncludeSafety: false})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal lookup envelope: %v", err)
	}
	if !strings.Contains(string(data), `"arguments":[`) || !strings.Contains(string(data), `"returns":[]`) {
		t.Fatalf("arguments/returns should be stable arrays for no-return functions: %s", string(data))
	}
}

func TestSearchBlizzardAPIFindsIndexedAPIs(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "Auction"})
	if !env.OK || !hasAPIResult(env.Data.Results, "C_AuctionHouse") || !hasAPIResult(env.Data.Results, "C_AuctionHouse.GetItemSearchResultInfo") {
		t.Fatalf("search envelope wrong: %#v", env)
	}
}

func TestSearchAndLookupBlizzardAPIIncludeDocumentedTables(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	search := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "ItemSearchResultInfo"})
	if !search.OK || !hasAPIResultOfType(search.Data.Results, "ItemSearchResultInfo", "Structure") {
		t.Fatalf("documented table missing from API search: %#v", search)
	}

	lookup := LookupBlizzardAPI(repo, idx, LookupAPIOptions{Name: "ItemSearchResultInfo", Exact: true})
	if !lookup.OK || lookup.Data.Name != "ItemSearchResultInfo" || lookup.Data.Type != "Structure" {
		t.Fatalf("documented table missing from API lookup: %#v", lookup)
	}
}

func TestSearchBlizzardAPIJSONUsesEmptyArrayForNoResults(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "definitely_missing_api"})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal search envelope: %v", err)
	}
	if strings.Contains(string(data), `"results":null`) || !strings.Contains(string(data), `"results":[]`) {
		t.Fatalf("empty search results should marshal as an array: %s", string(data))
	}
}

func TestSearchBlizzardAPIFiltersTypeLimitAndSafety(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	events := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "Auction", Type: "Event"})
	if !events.OK || len(events.Data.Results) != 1 || events.Data.Results[0].Type != "Event" {
		t.Fatalf("type-filtered search envelope wrong: %#v", events)
	}

	limited := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "Auction", Limit: 1})
	if !limited.OK || len(limited.Data.Results) != 1 {
		t.Fatalf("limited search envelope wrong: %#v", limited)
	}

	protected := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "Auction", Safety: string(analyze.RiskProtected)})
	if !protected.OK || len(protected.Data.Results) != 1 || protected.Data.Results[0].Safety == nil || protected.Data.Results[0].Safety.Classification.Level != analyze.RiskProtected {
		t.Fatalf("safety-filtered search envelope wrong: %#v", protected)
	}
}

func TestSearchBlizzardAPIMatchesParametersReturnsAndSystems(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	argument := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "sorts", Type: "function"})
	if !argument.OK || !hasAPIResult(argument.Data.Results, "C_AuctionHouse.GetItemSearchResultInfo") {
		t.Fatalf("search should match argument names: %#v", argument)
	}

	returnType := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "ItemSearchResultInfo", Type: "function"})
	if !returnType.OK || !hasAPIResult(returnType.Data.Results, "C_AuctionHouse.GetItemSearchResultInfo") {
		t.Fatalf("search should match return types: %#v", returnType)
	}

	tableField := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "price", Type: "table"})
	if !tableField.OK || !hasAPIResultOfType(tableField.Data.Results, "ItemSearchResultInfo", "Structure") {
		t.Fatalf("search should match table fields: %#v", tableField)
	}
}

func TestSearchBlizzardAPIFiltersUnsafeOnly(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SearchBlizzardAPI(repo, idx, APISearchOptions{Query: "Auction", IncludeUnsafeOnly: true, Scenario: "combat"})
	if !env.OK {
		t.Fatalf("unsafe-only search envelope should be ok: %#v", env)
	}
	if len(env.Data.Results) != 2 || !hasAPIResult(env.Data.Results, "C_AuctionHouse.GetItemSearchResultInfo") || !hasAPIResult(env.Data.Results, "C_AuctionHouse.GetRestrictedInfo") {
		t.Fatalf("unsafe-only search should include only risky APIs: %#v", env.Data.Results)
	}
	for _, result := range env.Data.Results {
		if result.Safety == nil || result.Safety.Classification.Level == analyze.RiskSafe || result.Safety.Classification.Level == analyze.RiskNeverSecret {
			t.Fatalf("unsafe-only search returned safe result: %#v", result)
		}
	}
}

func TestGetAPINamespaceListReturnsOnlyNamespaces(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := GetAPINamespace(repo, idx, "list")
	if !env.OK {
		t.Fatalf("namespace list should be ok: %#v", env)
	}
	if len(env.Data.Results) != 1 || env.Data.Results[0].Name != "C_AuctionHouse" || env.Data.Results[0].Type != "System" {
		t.Fatalf("namespace=list should only return system namespaces: %#v", env.Data.Results)
	}
	if len(env.Data.Namespaces) != 1 || env.Data.Namespaces[0].Namespace != "C_AuctionHouse" || env.Data.Namespaces[0].FunctionCount != 4 {
		t.Fatalf("namespace summaries wrong: %#v", env.Data.Namespaces)
	}
}

func TestGetAPINamespaceReturnsStructuredGroups(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := GetAPINamespace(repo, idx, "C_AuctionHouse")
	if !env.OK {
		t.Fatalf("namespace lookup should be ok: %#v", env)
	}
	if env.Data.Namespace != "C_AuctionHouse" || len(env.Data.Systems) != 1 || env.Data.Systems[0] != "C_AuctionHouse" {
		t.Fatalf("namespace/system fields wrong: %#v", env.Data)
	}
	if len(env.Data.Functions) != 4 || len(env.Data.Events) != 1 || len(env.Data.Tables) != 1 {
		t.Fatalf("namespace groups wrong: functions=%#v events=%#v tables=%#v", env.Data.Functions, env.Data.Events, env.Data.Tables)
	}
	if len(env.Data.Results) != 7 {
		t.Fatalf("namespace should preserve combined results: %#v", env.Data.Results)
	}
}

func TestGetAPINamespaceJSONUsesEmptyArrayForNoResults(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := GetAPINamespace(repo, idx, "Definitely_Missing_Namespace")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal namespace envelope: %v", err)
	}
	if strings.Contains(string(data), `"results":null`) || !strings.Contains(string(data), `"results":[]`) {
		t.Fatalf("empty namespace results should marshal as an array: %s", string(data))
	}
}

func TestGetAPIEventsFiltersByNameOrPayload(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := GetAPIEvents(repo, idx, EventOptions{Event: "list", Filter: "isAethereal"})
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "AUCTION_HOUSE_SHOW" {
		t.Fatalf("event filter should match payload names: %#v", env)
	}

	missing := GetAPIEvents(repo, idx, EventOptions{Event: "list", Filter: "missing_payload"})
	if !missing.OK || len(missing.Data.Results) != 0 {
		t.Fatalf("event filter should return empty array for no matches: %#v", missing)
	}
}

func TestSearchFrameXMLFindsLineResults(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SearchFrameXML(repo, idx, FrameXMLSearchOptions{Query: "SecureActionButtonTemplate", MaxResults: 5})
	if !env.OK || len(env.Data.Results) < 1 || env.Data.Results[0].Line != 2 {
		t.Fatalf("framexml search envelope wrong: %#v", env)
	}
}

func TestSearchFrameXMLFiltersFilePatternAndIncludesContext(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SearchFrameXML(repo, idx, FrameXMLSearchOptions{
		Query:        "SecureActionButtonTemplate",
		FilePattern:  "SecureTemplates.lua",
		ContextLines: 1,
		MaxResults:   1,
	})
	if !env.OK || len(env.Data.Results) != 1 {
		t.Fatalf("framexml search envelope wrong: %#v", env)
	}
	result := env.Data.Results[0]
	if result.Line != 2 || len(result.Before) != 1 || len(result.After) != 1 {
		t.Fatalf("framexml context missing: %#v", result)
	}
	if !strings.Contains(result.Before[0], "setup helpers") || !strings.Contains(result.After[0], "SecureActionButtonMixin") {
		t.Fatalf("framexml context lines wrong: %#v", result)
	}
}

func TestSearchFrameXMLJSONUsesEmptyArrayForNoResults(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SearchFrameXML(repo, idx, FrameXMLSearchOptions{Query: "DefinitelyMissingTemplate"})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), `"results":null`) || !strings.Contains(string(data), `"results":[]`) {
		t.Fatalf("empty framexml results should marshal as an array: %s", string(data))
	}
}

func TestFindMixinTemplateFindsFrameXMLTemplates(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := FindMixinTemplate(repo, idx, "SecureActionButtonTemplate", "", 5)
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "SecureActionButtonTemplate" {
		t.Fatalf("template envelope wrong: %#v", env)
	}
}

func TestFindMixinTemplateSupportsKindFilter(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	template := FindMixinTemplate(repo, idx, "SecureActionButton", "Template", 5)
	if !template.OK || len(template.Data.Results) != 1 || template.Data.Results[0].Name != "SecureActionButtonTemplate" {
		t.Fatalf("template kind filter wrong: %#v", template)
	}

	mixin := FindMixinTemplate(repo, idx, "SecureActionButton", "Mixin", 5)
	if !mixin.OK || len(mixin.Data.Results) != 1 || mixin.Data.Results[0].Name != "SecureActionButtonMixin" {
		t.Fatalf("mixin kind filter wrong: %#v", mixin)
	}
}

func TestGetWowConstantsFindsEnumerations(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail-constants"), "retail-constants")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := GetWowConstants(repo, idx, ConstantsOptions{Name: "Enum.ItemQuality"})
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Name != "Enum.ItemQuality" {
		t.Fatalf("constants envelope wrong: %#v", env)
	}
	if len(env.Data.Results[0].Fields) != 2 || len(env.Data.Results[0].Values) != 2 || env.Data.Results[0].Values[0].Value == nil || *env.Data.Results[0].Values[0].Value != 0 {
		t.Fatalf("constants should include enum fields/values: %#v", env.Data.Results[0])
	}
}

func TestGetWowConstantsHonorsKindAndLimit(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail-constants"), "retail-constants")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	enum := GetWowConstants(repo, idx, ConstantsOptions{Name: "list", Kind: "Enumeration", Limit: 1})
	if !enum.OK || len(enum.Data.Results) != 1 || enum.Data.Results[0].Type != "Enumeration" {
		t.Fatalf("kind/limit constants envelope wrong: %#v", enum)
	}

	missing := GetWowConstants(repo, idx, ConstantsOptions{Name: "list", Kind: "Table", Limit: 1})
	if !missing.OK || len(missing.Data.Results) != 0 {
		t.Fatalf("kind filter should exclude non-matching constants: %#v", missing)
	}
}

func TestGetWowConstantsSupportsTextFilter(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail-constants"), "retail-constants")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	filtered := GetWowConstants(repo, idx, ConstantsOptions{Name: "list", Filter: "Power"})
	if !filtered.OK || len(filtered.Data.Results) != 1 || filtered.Data.Results[0].Name != "Enum.PowerType" {
		t.Fatalf("filter should narrow constants by name: %#v", filtered)
	}
}

func TestExplainAPISafetyReturnsScenarioEnvelope(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := ExplainAPISafety(repo, idx, "C_AuctionHouse.GetItemSearchResultInfo", "unit_cast")
	if !env.OK || env.Data.Explanation.Scenario != "unit_cast" || env.Data.Explanation.EffectiveLevel != analyze.RiskProtected {
		t.Fatalf("safety envelope wrong: %#v", env)
	}
	if !containsText(env.Data.Explanation.Why, "SecretWhenUnitSpellCastRestricted") {
		t.Fatalf("safety why should use indexed metadata: %#v", env.Data.Explanation.Why)
	}
}

func TestExplainAPISafetyTaintedScenarioMentionsUntaintedCaller(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := ExplainAPISafety(repo, idx, "C_AuctionHouse.GetItemSearchResultInfo", "tainted")
	if !env.OK || env.Data.Explanation.Scenario != "tainted" || env.Data.Explanation.EffectiveLevel != analyze.RiskProtected {
		t.Fatalf("tainted safety envelope wrong: %#v", env)
	}
	if !containsText(env.Data.Explanation.Why, "AllowedWhenUntainted") {
		t.Fatalf("tainted safety why should mention AllowedWhenUntainted: %#v", env.Data.Explanation.Why)
	}
	if !containsText(env.Data.Explanation.AddonAdvice, "untainted") {
		t.Fatalf("tainted safety advice should mention untainted caller: %#v", env.Data.Explanation.AddonAdvice)
	}
}

func TestExplainAPISafetyIncludesRawMetadataAndClassification(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := ExplainAPISafety(repo, idx, "C_AuctionHouse.GetRestrictedInfo", "unit_cast")
	if !env.OK {
		t.Fatalf("safety envelope should be ok: %#v", env)
	}
	if env.Data.Raw.SecretWrapperConstant != "ContextuallySecret" || len(env.Data.Raw.RestrictedTypes) != 2 {
		t.Fatalf("raw safety metadata missing: %#v", env.Data.Raw)
	}
	if env.Data.Classification.Level != analyze.RiskConditionalSecret || !hasSafetyField(env.Data.Classification.Fields, "target", true, false) {
		t.Fatalf("classification missing field-level status: %#v", env.Data.Classification)
	}
}

func TestExplainAPISafetyJSONUsesEmptyArraysForSafetySlices(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := ExplainAPISafety(repo, idx, "ItemSearchResultInfo", "default")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal safety envelope: %v", err)
	}
	for _, unexpected := range []string{`"SecretArgumentsAddAspect":null`, `"SecretReturnsForAspect":null`, `"RestrictedTypes":null`, `"Fields":null`} {
		if strings.Contains(string(data), unexpected) {
			t.Fatalf("safety slices should marshal as empty arrays, found %s in %s", unexpected, string(data))
		}
	}
}

func TestRemainingToolEnvelopesAreStructured(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(retail)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	events := GetAPIEvents(retail, idx, EventOptions{Event: "list"})
	if !events.OK || !hasAPIResult(events.Data.Results, "AUCTION_HOUSE_SHOW") {
		t.Fatalf("events envelope should include fixture event: %#v", events)
	}
	for _, result := range events.Data.Results {
		if result.Name == "AUCTION_HOUSE_SHOW" {
			if result.Signature != "AUCTION_HOUSE_SHOW(auctionHouseID, isAethereal)" || len(result.Arguments) != 2 {
				t.Fatalf("event payload details missing: %#v", result)
			}
		}
	}

	deprecation := CheckAPIDeprecation(retail, "GetContainerItemInfo()")
	if !deprecation.OK {
		t.Fatalf("deprecation envelope should be ok: %#v", deprecation)
	}

	migration := SuggestAPIMigration(retail, "GetContainerItemInfo")
	if !migration.OK || migration.Data.OldFunction != "GetContainerItemInfo" {
		t.Fatalf("migration envelope wrong: %#v", migration)
	}

	widget := GetWidgetAPI(retail, idx, "Button")
	if !widget.OK || widget.Data.WidgetType != "Button" || !hasAPIResult(widget.Data.Results, "Button.SetText") {
		t.Fatalf("widget envelope wrong: %#v", widget)
	}
	for _, result := range widget.Data.Results {
		if result.Name == "Button.SetText" && result.Signature != "Button.SetText(text, isFormatted)" {
			t.Fatalf("widget method signature missing: %#v", result)
		}
	}
	widgetList := GetWidgetAPI(retail, idx, "list")
	if !widgetList.OK || !hasAPIResult(widgetList.Data.Results, "Button") {
		t.Fatalf("widget list wrong: %#v", widgetList)
	}

	cvars := LookupCVar(retail, idx, CVarLookupOptions{Name: "graphicsQuality"})
	if !cvars.OK || len(cvars.Data.Results) != 1 || cvars.Data.Results[0].Name != "graphicsQuality" {
		t.Fatalf("cvar envelope wrong: %#v", cvars)
	}

	toc := ValidateTOC("## Interface: 120000\n## Title: My Addon\nmain.lua\n", "", "My Addon")
	if !toc.OK || toc.Data.AddonName != "My Addon" || len(toc.Data.Errors) != 0 {
		t.Fatalf("toc envelope wrong: %#v", toc)
	}
}

func TestSuggestAPIMigrationWarnsForClassicLikeClients(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	classic := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "partial-classic")

	env := SuggestAPIMigration(classic, "GetContainerItemInfo")
	if !env.OK || len(env.Data.Suggestions) == 0 {
		t.Fatalf("migration envelope wrong: %#v", env)
	}
	if !containsText(env.Data.Warnings, "Classic") {
		t.Fatalf("classic-like client should warn about replacement availability: %#v", env.Data.Warnings)
	}
}

func TestCheckAPIDeprecationJSONUsesEmptyArrayForNoMatches(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	env := CheckAPIDeprecation(retail, "print('modern code path')")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), `"deprecated":null`) || !strings.Contains(string(data), `"deprecated":[]`) {
		t.Fatalf("empty deprecated should marshal as an array: %s", string(data))
	}
}

func TestSuggestAPIMigrationJSONUsesEmptyArrayForUnknownFunction(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	env := SuggestAPIMigration(retail, "DefinitelyUnknownFunction")
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), `"suggestions":null`) || !strings.Contains(string(data), `"suggestions":[]`) {
		t.Fatalf("empty suggestions should marshal as an array: %s", string(data))
	}
}

func TestCheckAPIDeprecationValidatesReplacementAgainstIndex(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(retail)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	missing := CheckAPIDeprecationWithIndex(retail, idx, "GetContainerItemInfo(0, 1)")
	if !missing.OK || !containsText(missing.Data.Warnings, "C_Container.GetContainerItemInfo is not documented") {
		t.Fatalf("missing replacement warning not returned: %#v", missing)
	}

	idx.APIs["C_Container.GetContainerItemInfo"] = analyze.APIEntry{Name: "C_Container.GetContainerItemInfo", Type: "Function"}
	available := CheckAPIDeprecationWithIndex(retail, idx, "GetContainerItemInfo(0, 1)")
	if containsText(available.Data.Warnings, "C_Container.GetContainerItemInfo is not documented") {
		t.Fatalf("available replacement should not warn: %#v", available.Data.Warnings)
	}
}

func TestSuggestAPIMigrationValidatesReplacementAgainstIndex(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(retail)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := SuggestAPIMigrationWithIndex(retail, idx, "GetContainerItemInfo")
	if !env.OK || !containsText(env.Data.Warnings, "C_Container.GetContainerItemInfo is not documented") {
		t.Fatalf("migration should warn when replacement is absent from selected source: %#v", env)
	}
}

func TestCheckAPIDeprecationWarnsForClassicLikeClients(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	classic := analyze.DetectRepository(filepath.Join(root, "partial-classic"), "partial-classic")

	env := CheckAPIDeprecation(classic, "GetContainerItemInfo(0, 1)")
	if !env.OK || len(env.Data.Deprecated) != 1 || env.Data.Deprecated[0] != "GetContainerItemInfo" {
		t.Fatalf("deprecation envelope wrong: %#v", env)
	}
	if !containsText(env.Data.Warnings, "Classic") {
		t.Fatalf("classic-like deprecation warning missing: %#v", env.Data.Warnings)
	}
}

func TestLookupCVarHonorsDetailOption(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(retail)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	summary := LookupCVar(retail, idx, CVarLookupOptions{Name: "graphicsQuality"})
	if !summary.OK || len(summary.Data.Results) != 1 {
		t.Fatalf("cvar summary envelope wrong: %#v", summary)
	}
	if summary.Data.Results[0].DefaultValue != "5" || summary.Data.Results[0].Description != "" || summary.Data.Results[0].References == 0 || len(summary.Data.Results[0].Files) == 0 {
		t.Fatalf("summary should include default/reference/file details but omit description: %#v", summary.Data.Results[0])
	}

	detail := LookupCVar(retail, idx, CVarLookupOptions{Name: "graphicsQuality", Detail: true})
	if !detail.OK || len(detail.Data.Results) != 1 {
		t.Fatalf("cvar detail envelope wrong: %#v", detail)
	}
	if detail.Data.Results[0].DefaultValue != "5" || detail.Data.Results[0].Description == "" || detail.Data.Results[0].Line == 0 || detail.Data.Results[0].Usage == "" {
		t.Fatalf("detail=true should include reference details: %#v", detail.Data.Results[0])
	}
}

func TestLookupCVarJSONUsesEmptyArrayForNoResults(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	retail := analyze.DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	idx, err := analyze.BuildIndex(retail)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	env := LookupCVar(retail, idx, CVarLookupOptions{Name: "definitely_missing_cvar"})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal cvar envelope: %v", err)
	}
	if strings.Contains(string(data), `"results":null`) || !strings.Contains(string(data), `"results":[]`) {
		t.Fatalf("empty cvar results should marshal as an array: %s", string(data))
	}
}

func TestValidateTOCReadsTOCPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MyAddon.toc")
	if err := os.WriteFile(path, []byte("## Title: My Addon\nmain.lua\n"), 0o600); err != nil {
		t.Fatalf("write toc fixture: %v", err)
	}
	env := ValidateTOC("", path, "MyAddon")
	if !env.OK || len(env.Data.Errors) != 1 || !containsText(env.Data.Errors, "Missing required field: ## Interface") {
		t.Fatalf("toc path validation did not inspect file content: %#v", env)
	}
}

func TestValidateTOCWarnsWhenInterfaceDoesNotMatchSourceVersion(t *testing.T) {
	env := ValidateTOCWithOptions("## Interface: 11507\n## Title: My Addon\n", "", "MyAddon", TOCValidationOptions{
		SourceVersion: "12.0.0.60000",
	})
	if !env.OK {
		t.Fatalf("toc validation should be ok: %#v", env)
	}
	if !containsText(env.Data.Warnings, "12.0.0.60000") {
		t.Fatalf("source-aware interface warning missing: %#v", env.Data.Warnings)
	}
}

func hasAPIResult(results []APIResult, name string) bool {
	for _, result := range results {
		if result.Name == name {
			return true
		}
	}
	return false
}

func hasAPIResultOfType(results []APIResult, name, typ string) bool {
	for _, result := range results {
		if result.Name == name && result.Type == typ {
			return true
		}
	}
	return false
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func hasSafetyField(fields []analyze.SafetyField, name string, conditionalSecret, neverSecret bool) bool {
	for _, field := range fields {
		if field.Name == name && field.ConditionalSecret == conditionalSecret && field.NeverSecret == neverSecret {
			return true
		}
	}
	return false
}

type remoteRefsFixtureGit map[string]string

func (g remoteRefsFixtureGit) Run(args ...string) error { return nil }

func (g remoteRefsFixtureGit) Output(args ...string) ([]byte, error) {
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

type failingRemoteRefsGit struct{}

func (failingRemoteRefsGit) Run(args ...string) error { return os.ErrNotExist }
func (failingRemoteRefsGit) Output(args ...string) ([]byte, error) {
	return nil, os.ErrNotExist
}
