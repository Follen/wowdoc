package analyze

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIndexFindsAPINamesInValidRetail(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api, ok := idx.APIs["C_AuctionHouse"]
	if !ok {
		t.Fatalf("expected C_AuctionHouse API namespace in index: %#v", idx.APIs)
	}
	if api.Name != "C_AuctionHouse" || api.Type != "System" {
		t.Fatalf("unexpected API namespace entry: %#v", api)
	}
}

func TestBuildIndexUsesBoundedWorkerPoolForFileScanning(t *testing.T) {
	source, err := os.ReadFile("index.go")
	if err != nil {
		t.Fatalf("read index.go: %v", err)
	}
	text := string(source)
	for _, required := range []string{"const indexWorkerCount", "jobs := make(chan indexJob)", "results := make(chan indexResult)", "for worker := 0; worker < indexWorkerCount; worker++"} {
		if !strings.Contains(text, required) {
			t.Fatalf("BuildIndex should use a bounded worker pool; missing %q", required)
		}
	}
}

func TestBuildIndexParsesAPIFunctionSignature(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api, ok := idx.APIs["C_AuctionHouse.GetItemSearchResultInfo"]
	if !ok {
		t.Fatalf("expected function API in index: %#v", idx.APIs)
	}
	if api.Signature != "C_AuctionHouse.GetItemSearchResultInfo(itemKey, sorts) -> itemSearchResultInfo" {
		t.Fatalf("signature = %q", api.Signature)
	}
	if len(api.Arguments) != 2 || api.Arguments[0].Name != "itemKey" || api.Arguments[1].Nilable != true {
		t.Fatalf("arguments not parsed: %#v", api.Arguments)
	}
	if len(api.Returns) != 1 || api.Returns[0].Name != "itemSearchResultInfo" || !api.Returns[0].Nilable {
		t.Fatalf("returns not parsed: %#v", api.Returns)
	}
}

func TestBuildIndexParsesDocumentedTableFields(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	table, ok := idx.APIs["ItemSearchResultInfo"]
	if !ok {
		t.Fatalf("expected documented table in index: %#v", idx.APIs)
	}
	if table.Namespace != "C_AuctionHouse" || table.System != "C_AuctionHouse" {
		t.Fatalf("table namespace/system not parsed: %#v", table)
	}
	if len(table.Fields) != 2 || table.Fields[0].Name != "itemID" || table.Fields[1].Name != "price" {
		t.Fatalf("table fields not parsed: %#v", table.Fields)
	}
}

func TestBuildIndexParsesConstantEnumValues(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail-constants"), "retail-constants")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	enum, ok := idx.Constants["Enum.ItemQuality"]
	if !ok {
		t.Fatalf("expected Enum.ItemQuality in constants index: %#v", idx.Constants)
	}
	if len(enum.Fields) != 2 || enum.Fields[0].Name != "Poor" || enum.Fields[0].Type != "Enum.ItemQuality" {
		t.Fatalf("enum fields not parsed: %#v", enum.Fields)
	}
	if len(enum.Values) != 2 || enum.Values[0].Value == nil || *enum.Values[0].Value != 0 || enum.Values[1].Value == nil || *enum.Values[1].Value != 1 {
		t.Fatalf("enum values not parsed: %#v", enum.Values)
	}
}

func TestBuildIndexPreservesAPISafetyMetadata(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api := idx.APIs["C_AuctionHouse.GetItemSearchResultInfo"]
	if !api.Safety.IsProtectedFunction || api.Safety.SecretArguments != "AllowedWhenUntainted" || !api.Safety.SecretWhenUnitSpellCastRestricted {
		t.Fatalf("safety metadata not parsed: %#v", api.Safety)
	}
	if got := ClassifySafety(api.Safety).Level; got != RiskProtected {
		t.Fatalf("safety level = %q, want %q", got, RiskProtected)
	}
}

func TestBuildIndexParsesAdvancedSafetyMetadata(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api, ok := idx.APIs["C_AuctionHouse.GetRestrictedInfo"]
	if !ok {
		t.Fatalf("expected function API in index: %#v", idx.APIs)
	}
	if api.Safety.SecretWrapperConstant != "ContextuallySecret" {
		t.Fatalf("SecretWrapperConstant = %q", api.Safety.SecretWrapperConstant)
	}
	if !api.Safety.IsPreventingSecretValues {
		t.Fatalf("IsPreventingSecretValues not parsed: %#v", api.Safety)
	}
	if got, want := api.Safety.SecretArgumentsAddAspect, []string{"UnitTokenRestrictedForAddOns"}; !sameStrings(got, want) {
		t.Fatalf("SecretArgumentsAddAspect = %#v, want %#v", got, want)
	}
	if got, want := api.Safety.SecretReturnsForAspect, []string{"UnitTokenPvPRestrictedForAddOns"}; !sameStrings(got, want) {
		t.Fatalf("SecretReturnsForAspect = %#v, want %#v", got, want)
	}
	if got, want := api.Safety.RestrictedTypes, []string{"UnitTokenRestrictedForAddOns", "UnitTokenPvPRestrictedForAddOns"}; !sameStrings(got, want) {
		t.Fatalf("RestrictedTypes = %#v, want %#v", got, want)
	}
	if !hasSafetyField(api.Safety.Fields, "target", true, false) {
		t.Fatalf("target field safety not parsed: %#v", api.Safety.Fields)
	}
	if !hasSafetyField(api.Safety.Fields, "castBarID", false, true) {
		t.Fatalf("castBarID field safety not parsed: %#v", api.Safety.Fields)
	}
	if got := ClassifySafety(api.Safety).Level; got != RiskConditionalSecret {
		t.Fatalf("safety level = %q, want %q", got, RiskConditionalSecret)
	}
}

func TestBuildIndexParsesAPIFunctionWithoutReturns(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api, ok := idx.APIs["C_AuctionHouse.StartCommoditiesPurchase"]
	if !ok {
		t.Fatalf("expected no-return function API in index: %#v", idx.APIs)
	}
	if api.Signature != "C_AuctionHouse.StartCommoditiesPurchase(itemID, quantity)" {
		t.Fatalf("signature = %q", api.Signature)
	}
	if len(api.Arguments) != 2 || len(api.Returns) != 0 {
		t.Fatalf("arguments/returns not parsed: args=%#v returns=%#v", api.Arguments, api.Returns)
	}
}

func TestBuildIndexParsesAPIFunctionWithNilReturns(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api, ok := idx.APIs["C_AuctionHouse.CancelCommoditiesPurchase"]
	if !ok {
		t.Fatalf("expected nil-return function API in index: %#v", idx.APIs)
	}
	if api.Signature != "C_AuctionHouse.CancelCommoditiesPurchase(itemID)" {
		t.Fatalf("signature = %q", api.Signature)
	}
	if len(api.Arguments) != 1 || len(api.Returns) != 0 {
		t.Fatalf("arguments/returns not parsed: args=%#v returns=%#v", api.Arguments, api.Returns)
	}
}

func TestBuildIndexDoesNotTreatParamsOrWidgetsAsTopLevelAPIs(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	for _, unexpected := range []string{"target", "castBarID", "quantity", "Button", "GetRestrictedInfo", "StartCommoditiesPurchase"} {
		if _, ok := idx.APIs[unexpected]; ok {
			t.Fatalf("unexpected top-level API entry leaked from nested documentation: %q => %#v", unexpected, idx.APIs[unexpected])
		}
	}
}

func TestBuildIndexParsesAPIEventsWithPayload(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	event, ok := idx.APIs["AUCTION_HOUSE_SHOW"]
	if !ok {
		t.Fatalf("expected event in index: %#v", idx.APIs)
	}
	if event.Type != "Event" || event.Signature != "AUCTION_HOUSE_SHOW(auctionHouseID, isAethereal)" {
		t.Fatalf("event signature wrong: %#v", event)
	}
	if len(event.Arguments) != 2 || event.Arguments[0].Name != "auctionHouseID" || event.Arguments[0].Type != "number" || event.Arguments[1].Name != "isAethereal" || event.Arguments[1].Type != "bool" {
		t.Fatalf("event payload not parsed: %#v", event.Arguments)
	}
}

func TestBuildIndexParsesWidgetMethods(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	methods := idx.Widgets["Button"]
	if len(methods) != 1 {
		t.Fatalf("button widget methods = %#v", methods)
	}
	if methods[0].Name != "Button.SetText" || methods[0].Signature != "Button.SetText(text, isFormatted)" || len(methods[0].Arguments) != 2 || !methods[0].Arguments[1].Nilable {
		t.Fatalf("button method details wrong: %#v", methods[0])
	}
}

func TestBuildIndexParsesCVars(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	if !repo.Capabilities.CVars {
		t.Fatalf("fixture should expose CVar capability: %#v", repo.Capabilities)
	}

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	cvar, ok := idx.CVars["graphicsQuality"]
	if !ok {
		t.Fatalf("expected graphicsQuality CVar in index: %#v", idx.CVars)
	}
	if cvar.Name != "graphicsQuality" || cvar.DefaultValue != "5" || cvar.Description == "" {
		t.Fatalf("cvar details wrong: %#v", cvar)
	}
	sound, ok := idx.CVars["Sound_EnableSFX"]
	if !ok {
		t.Fatalf("expected Set/Get CVar references in index: %#v", idx.CVars)
	}
	if sound.DefaultValue != `"0"` || len(sound.References) != 2 {
		t.Fatalf("Set/Get CVar references wrong: %#v", sound)
	}
}

func TestBuildIndexFindsSecureActionButtonTemplateFrameXMLInPartialClassic(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	frame, ok := idx.FrameXML["SecureActionButtonTemplate"]
	if !ok {
		t.Fatalf("expected SecureActionButtonTemplate FrameXML in index: %#v", idx.FrameXML)
	}
	if frame.Name != "SecureActionButtonTemplate" || filepath.Base(frame.Path) != "SecureTemplates.lua" {
		t.Fatalf("unexpected FrameXML entry: %#v", frame)
	}
	results := idx.SearchFrameXML(FrameXMLSearchOptions{Query: "SecureActionButtonTemplate", MaxResults: 5})
	if len(results) < 1 {
		t.Fatalf("expected FrameXML search results, got %#v", results)
	}
	if filepath.Base(results[0].File) != "SecureTemplates.lua" || results[0].Line != 2 || results[0].Text == "" {
		t.Fatalf("search result must include file, line, and text: %#v", results[0])
	}
}

func TestBuildIndexFindsMixinEntriesInFrameXML(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	entry, ok := idx.FrameXML["SecureActionButtonMixin"]
	if !ok {
		t.Fatalf("expected SecureActionButtonMixin in FrameXML index: %#v", idx.FrameXML)
	}
	if entry.Kind != "Mixin" || filepath.Base(entry.Path) != "SecureTemplates.lua" || entry.Line != 3 {
		t.Fatalf("unexpected mixin entry: %#v", entry)
	}
}

func TestBuildIndexSearchesAllAddonLuaAndXMLFiles(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	results := idx.SearchFrameXML(FrameXMLSearchOptions{Query: "auctionhouseframe", FilePattern: "*.lua", MaxResults: 5})
	if len(results) != 1 || filepath.Base(results[0].File) != "TestAddon.lua" {
		t.Fatalf("expected case-insensitive AddOns search result from TestAddon.lua, got %#v", results)
	}
}

func TestBuildIndexParsesXMLTemplatesAndMixinInheritance(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	template, ok := idx.FrameXML["AuctionHouseFrame"]
	if !ok {
		t.Fatalf("expected XML template in index: %#v", idx.FrameXML)
	}
	if template.Kind != "Template" || template.Line != 2 || !sameStrings(template.Inherits, []string{"SecureActionButtonTemplate"}) {
		t.Fatalf("XML template details wrong: %#v", template)
	}

	mixin, ok := idx.FrameXML["TestAddonMixin"]
	if !ok {
		t.Fatalf("expected CreateFromMixins mixin in index: %#v", idx.FrameXML)
	}
	if mixin.Kind != "Mixin" || !sameStrings(mixin.Inherits, []string{"SecureActionButtonMixin"}) || mixin.Snippet == "" {
		t.Fatalf("mixin inheritance details wrong: %#v", mixin)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasSafetyField(fields []SafetyField, name string, conditionalSecret, neverSecret bool) bool {
	for _, field := range fields {
		if field.Name == name && field.ConditionalSecret == conditionalSecret && field.NeverSecret == neverSecret {
			return true
		}
	}
	return false
}
