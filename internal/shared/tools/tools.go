package tools

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
	"wowdoc/internal/shared/source"
)

type Handler interface{}

type ToolDefinition struct {
	Name         string
	SourceBacked bool
}

type Registry struct {
	Tools map[string]ToolDefinition
}

type ClientSummary struct {
	Alias        string               `json:"alias"`
	Version      string               `json:"version,omitempty"`
	Path         string               `json:"path,omitempty"`
	DefaultRef   string               `json:"defaultRef,omitempty"`
	RequestedRef string               `json:"requestedRef,omitempty"`
	ResolvedRef  string               `json:"resolvedRef,omitempty"`
	Capabilities analyze.Capabilities `json:"capabilities"`
}

type ListClientsData struct {
	Clients []ClientSummary `json:"clients"`
}

type ListClientsOptions struct {
	IncludeDiagnostics bool
	IncludeRefs        bool
	DefaultRefs        map[string]string
	DefaultRef         string
}

type RemoteRefInfo struct {
	Alias         string `json:"alias"`
	Repo          string `json:"repo"`
	ConfiguredRef string `json:"configuredRef"`
	RemoteRef     string `json:"remoteRef"`
	Commit        string `json:"commit,omitempty"`
	Version       string `json:"version,omitempty"`
	Path          string `json:"path,omitempty"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

type RemoteRefsData struct {
	Clients []RemoteRefInfo `json:"clients"`
}

type RemoteVersionResolver func(alias string) (version, path string, err error)

type APIResult struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace,omitempty"`
	System    string             `json:"system,omitempty"`
	Type      string             `json:"type,omitempty"`
	Path      string             `json:"path,omitempty"`
	Signature string             `json:"signature,omitempty"`
	Arguments []analyze.APIParam `json:"arguments"`
	Returns   []analyze.APIParam `json:"returns"`
	Fields    []analyze.APIParam `json:"fields,omitempty"`
	Values    []analyze.APIValue `json:"values,omitempty"`
	Safety    *APISafetyResult   `json:"safety,omitempty"`
}

func (r APIResult) MarshalJSON() ([]byte, error) {
	type apiResultJSON APIResult
	out := apiResultJSON(r)
	out.Arguments = stableAPIParams(out.Arguments)
	out.Returns = stableAPIParams(out.Returns)
	if out.Fields != nil {
		out.Fields = stableAPIParams(out.Fields)
	}
	return json.Marshal(out)
}

type LookupAPIOptions struct {
	Name          string `json:"name,omitempty"`
	Exact         bool   `json:"exact,omitempty"`
	IncludeSafety bool   `json:"includeSafety,omitempty"`
}

type APISafetyResult struct {
	Raw            analyze.SafetyMetadata       `json:"raw,omitempty"`
	Classification analyze.SafetyClassification `json:"classification,omitempty"`
}

type APISearchData struct {
	Results    []APIResult       `json:"results"`
	Namespace  string            `json:"namespace,omitempty"`
	Systems    []string          `json:"systems,omitempty"`
	Functions  []APIResult       `json:"functions,omitempty"`
	Events     []APIResult       `json:"events,omitempty"`
	Tables     []APIResult       `json:"tables,omitempty"`
	Namespaces []NamespaceResult `json:"namespaces,omitempty"`
}

type NamespaceResult struct {
	Namespace     string `json:"namespace"`
	FunctionCount int    `json:"functionCount"`
	SystemCount   int    `json:"systemCount,omitempty"`
}

type APISearchOptions struct {
	Query             string `json:"query,omitempty"`
	Type              string `json:"type,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	Safety            string `json:"safety,omitempty"`
	Scenario          string `json:"scenario,omitempty"`
	IncludeUnsafeOnly bool   `json:"includeUnsafeOnly,omitempty"`
}

type FrameXMLSearchData struct {
	Results []analyze.SearchResult `json:"results"`
}

type FrameXMLSearchOptions struct {
	Query        string `json:"query,omitempty"`
	FilePattern  string `json:"filePattern,omitempty"`
	ContextLines int    `json:"contextLines,omitempty"`
	MaxResults   int    `json:"maxResults,omitempty"`
}

type MixinTemplateResult struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Line     int      `json:"line,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Inherits []string `json:"inherits,omitempty"`
	Snippet  string   `json:"snippet,omitempty"`
}

type MixinTemplateData struct {
	Results []MixinTemplateResult `json:"results"`
}

type ConstantResult struct {
	Name   string             `json:"name"`
	Type   string             `json:"type,omitempty"`
	Path   string             `json:"path,omitempty"`
	System string             `json:"system,omitempty"`
	Fields []analyze.APIParam `json:"fields,omitempty"`
	Values []analyze.APIValue `json:"values,omitempty"`
}

type ConstantsData struct {
	Results []ConstantResult `json:"results"`
}

type ConstantsOptions struct {
	Name   string `json:"name,omitempty"`
	Filter string `json:"filter,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type SafetyExplanationData struct {
	Raw            analyze.SafetyMetadata       `json:"raw"`
	Classification analyze.SafetyClassification `json:"classification"`
	Explanation    analyze.SafetyExplanation    `json:"explanation"`
}

type EventData struct {
	Results []APIResult `json:"results"`
}

type EventOptions struct {
	Event  string `json:"event,omitempty"`
	Filter string `json:"filter,omitempty"`
}

type DeprecationData struct {
	Deprecated  []string             `json:"deprecated"`
	Details     []DeprecationFinding `json:"details,omitempty"`
	UnknownAPIs []string             `json:"unknownApis,omitempty"`
	Summary     string               `json:"summary,omitempty"`
	Warnings    []string             `json:"warnings,omitempty"`
}

type DeprecationFinding struct {
	Function    string `json:"function"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	Patch       string `json:"patch,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

type MigrationData struct {
	OldFunction string   `json:"oldFunction"`
	Suggestions []string `json:"suggestions"`
	Replacement string   `json:"replacement,omitempty"`
	Patch       string   `json:"patch,omitempty"`
	Notes       string   `json:"notes,omitempty"`
	CodeExample string   `json:"codeExample,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type WidgetAPIData struct {
	WidgetType string      `json:"widgetType"`
	Results    []APIResult `json:"results"`
}

type CVarData struct {
	Results []CVarResult `json:"results"`
}

type CVarLookupOptions struct {
	Name   string `json:"name,omitempty"`
	Detail bool   `json:"detail,omitempty"`
}

type CVarResult struct {
	Name         string   `json:"name"`
	DefaultValue string   `json:"defaultValue,omitempty"`
	Description  string   `json:"description,omitempty"`
	Path         string   `json:"path,omitempty"`
	File         string   `json:"file,omitempty"`
	Line         int      `json:"line,omitempty"`
	Usage        string   `json:"usage,omitempty"`
	Snippet      string   `json:"snippet,omitempty"`
	References   int      `json:"references,omitempty"`
	Files        []string `json:"files,omitempty"`
}

type TOCValidationData struct {
	AddonName string    `json:"addonName,omitempty"`
	Valid     bool      `json:"valid"`
	Warnings  []string  `json:"warnings,omitempty"`
	Errors    []string  `json:"errors,omitempty"`
	Info      []string  `json:"info,omitempty"`
	Parsed    TOCParsed `json:"parsed,omitempty"`
}

type TOCParsed struct {
	InterfaceVersions          []string          `json:"interfaceVersions"`
	Title                      string            `json:"title,omitempty"`
	SavedVariables             []string          `json:"savedVariables,omitempty"`
	SavedVariablesPerCharacter []string          `json:"savedVariablesPerCharacter,omitempty"`
	Dependencies               []string          `json:"dependencies,omitempty"`
	OptionalDeps               []string          `json:"optionalDeps,omitempty"`
	Files                      []string          `json:"files"`
	Metadata                   map[string]string `json:"metadata"`
}

type TOCValidationOptions struct {
	SourceVersion string
}

type knownMigration struct {
	Replacement string
	Patch       string
	Notes       string
}

var knownMigrations = map[string]knownMigration{
	"GetContainerItemInfo":     {Replacement: "C_Container.GetContainerItemInfo", Patch: "10.0.0", Notes: "Returns a table instead of multiple values"},
	"GetContainerNumSlots":     {Replacement: "C_Container.GetContainerNumSlots", Patch: "10.0.0"},
	"GetContainerItemLink":     {Replacement: "C_Container.GetContainerItemLink", Patch: "10.0.0"},
	"GetContainerNumFreeSlots": {Replacement: "C_Container.GetContainerNumFreeSlots", Patch: "10.0.0"},
	"GetContainerItemID":       {Replacement: "C_Container.GetContainerItemID", Patch: "10.0.0"},
	"UseContainerItem":         {Replacement: "C_Container.UseContainerItem", Patch: "10.0.0"},
	"PickupContainerItem":      {Replacement: "C_Container.PickupContainerItem", Patch: "10.0.0"},
	"SplitContainerItem":       {Replacement: "C_Container.SplitContainerItem", Patch: "10.0.0"},
	"GetSpellInfo":             {Replacement: "C_Spell.GetSpellInfo", Patch: "11.0.0", Notes: "Returns a table instead of multiple values"},
	"GetSpellName":             {Replacement: "C_Spell.GetSpellName", Patch: "11.0.0"},
	"GetSpellTexture":          {Replacement: "C_Spell.GetSpellTexture", Patch: "11.0.0"},
	"GetSpellCooldown":         {Replacement: "C_Spell.GetSpellCooldown", Patch: "11.0.0", Notes: "Returns a SpellCooldownInfo table"},
	"GetSpellCharges":          {Replacement: "C_Spell.GetSpellCharges", Patch: "11.0.0"},
	"GetSpellCount":            {Replacement: "C_Spell.GetSpellCount", Patch: "11.0.0"},
	"IsSpellKnown":             {Replacement: "C_SpellBook.IsSpellInSpellBook", Patch: "11.0.0"},
	"IsUsableSpell":            {Replacement: "C_Spell.IsSpellUsable", Patch: "11.0.0"},
	"IsSpellInRange":           {Replacement: "C_Spell.IsSpellInRange", Patch: "11.0.0"},
	"UnitBuff":                 {Replacement: "C_UnitAuras.GetBuffDataByIndex", Patch: "10.0.0", Notes: "Returns AuraData table"},
	"UnitDebuff":               {Replacement: "C_UnitAuras.GetDebuffDataByIndex", Patch: "10.0.0", Notes: "Returns AuraData table"},
	"UnitAura":                 {Replacement: "C_UnitAuras.GetAuraDataByIndex", Patch: "10.0.0", Notes: "Returns AuraData table"},
	"GetItemInfo":              {Replacement: "C_Item.GetItemInfo", Patch: "11.0.0", Notes: "Returns a table; use C_Item.GetItemInfoInstant for cached data"},
	"GetItemInfoInstant":       {Replacement: "C_Item.GetItemInfoInstant", Patch: "11.0.0"},
	"GetCurrencyInfo":          {Replacement: "C_CurrencyInfo.GetCurrencyInfo", Patch: "8.0.1"},
	"GetCurrencyListInfo":      {Replacement: "C_CurrencyInfo.GetCurrencyListInfo", Patch: "8.0.1"},
	"GetNumAddOns":             {Replacement: "C_AddOns.GetNumAddOns", Patch: "11.0.0"},
	"GetAddOnInfo":             {Replacement: "C_AddOns.GetAddOnInfo", Patch: "11.0.0"},
	"IsAddOnLoaded":            {Replacement: "C_AddOns.IsAddOnLoaded", Patch: "11.0.0"},
	"EnableAddOn":              {Replacement: "C_AddOns.EnableAddOn", Patch: "11.0.0"},
	"DisableAddOn":             {Replacement: "C_AddOns.DisableAddOn", Patch: "11.0.0"},
	"LoadAddOn":                {Replacement: "C_AddOns.LoadAddOn", Patch: "11.0.0"},
	"GameTooltip:SetBagItem":   {Replacement: "C_TooltipInfo.GetBagItem + GameTooltip:ProcessInfo", Patch: "10.0.2"},
	"GetAddOnMetadata":         {Replacement: "C_AddOns.GetAddOnMetadata", Patch: "11.0.0"},
	"GetBuildInfo":             {Replacement: "Still valid but check for 12.x changes", Patch: "current"},
}

var knownInterfaceVersions = map[string]string{
	"120000": "Midnight (12.0.0)",
	"110207": "The War Within 11.2.7",
	"110205": "The War Within 11.2.5",
	"110200": "The War Within 11.2.0",
	"110105": "The War Within 11.1.5",
	"110100": "The War Within 11.1.0",
	"110007": "The War Within 11.0.7",
	"110005": "The War Within 11.0.5",
	"110002": "The War Within 11.0.2",
	"110000": "The War Within 11.0.0",
	"100207": "Dragonflight 10.2.7",
	"100200": "Dragonflight 10.2.0",
	"100100": "Dragonflight 10.1.0",
	"100007": "Dragonflight 10.0.7",
	"100005": "Dragonflight 10.0.5",
	"100002": "Dragonflight 10.0.2",
	"40402":  "Cataclysm Classic 4.4.2",
	"40401":  "Cataclysm Classic 4.4.1",
	"40400":  "Cataclysm Classic 4.4.0",
	"30403":  "Wrath Classic 3.4.3",
	"11507":  "Classic Era 1.15.7",
	"11506":  "Classic Era 1.15.6",
	"11505":  "Classic Era 1.15.5",
}

func ListClients(repos []analyze.Repository, options ListClientsOptions) contracts.Envelope[ListClientsData] {
	env := contracts.Envelope[ListClientsData]{OK: true, Data: ListClientsData{Clients: []ClientSummary{}}}
	for _, repo := range repos {
		if repo.Valid {
			client := ClientSummary{
				Alias:        repo.Alias,
				Version:      repo.Version,
				Path:         repo.Path,
				Capabilities: repo.Capabilities,
			}
			if options.IncludeRefs {
				client.DefaultRef = options.DefaultRefs[repo.Alias]
				if client.DefaultRef == "" {
					client.DefaultRef = repo.RequestedRef
				}
				if client.DefaultRef == "" {
					client.DefaultRef = options.DefaultRef
				}
				client.RequestedRef = repo.RequestedRef
				client.ResolvedRef = repo.ResolvedRef
			}
			env.Data.Clients = append(env.Data.Clients, client)
			if options.IncludeDiagnostics {
				env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
			}
			continue
		}
		if options.IncludeDiagnostics {
			env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
		}
	}
	return env
}

func InspectRemoteRefs(repos, refs map[string]string, git source.GitRunner, versionResolver RemoteVersionResolver) contracts.Envelope[RemoteRefsData] {
	env := contracts.Envelope[RemoteRefsData]{OK: true, Data: RemoteRefsData{Clients: []RemoteRefInfo{}}}
	aliases := make([]string, 0, len(repos))
	for alias := range repos {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		ref := refs[alias]
		info := RemoteRefInfo{
			Alias:         alias,
			Repo:          repos[alias],
			ConfiguredRef: ref,
			RemoteRef:     ref,
			Status:        "unknown",
		}
		if git == nil {
			info = remoteRefFallback(info, versionResolver, "git is not configured")
			env.Data.Clients = append(env.Data.Clients, info)
			continue
		}
		out, err := git.Output("ls-remote", "--heads", info.Repo, ref)
		if err != nil {
			info = remoteRefFallback(info, versionResolver, err.Error())
			env.Data.Clients = append(env.Data.Clients, info)
			continue
		}
		commit := parseLsRemoteCommit(string(out), ref)
		if commit == "" {
			info = remoteRefFallback(info, versionResolver, "remote branch not found")
			env.Data.Clients = append(env.Data.Clients, info)
			continue
		}
		info.Commit = commit
		info.Status = "ok"
		if versionResolver != nil {
			version, path, err := versionResolver(alias)
			if err != nil {
				info.Status = "error"
				info.Error = err.Error()
			} else {
				info.Version = version
				info.Path = path
			}
		}
		env.Data.Clients = append(env.Data.Clients, info)
	}
	return env
}

func remoteRefFallback(info RemoteRefInfo, versionResolver RemoteVersionResolver, message string) RemoteRefInfo {
	if versionResolver == nil {
		info.Status = "error"
		info.Error = message
		return info
	}
	version, path, err := versionResolver(info.Alias)
	if err != nil {
		info.Status = "error"
		info.Error = err.Error()
		return info
	}
	info.Status = "fallback"
	info.Version = version
	info.Path = path
	return info
}

func parseLsRemoteCommit(output, ref string) string {
	want := "refs/heads/" + ref
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == want {
			return fields[0]
		}
	}
	return ""
}

func LookupBlizzardAPI(repo analyze.Repository, idx *analyze.Index, options LookupAPIOptions) contracts.Envelope[APIResult] {
	source := sourceFor(repo)
	if !repo.Capabilities.APIDocumentation {
		return withRepoDiagnostics(repo, contracts.Envelope[APIResult]{
			OK:     false,
			Source: source,
			Error: &contracts.ToolError{
				Code:    contracts.ErrCapabilityUnavailable,
				Message: "API documentation is unavailable for this source",
			},
		})
	}
	if idx != nil {
		if entry, ok := lookupAPIEntry(idx, options); ok {
			return withRepoDiagnostics(repo, contracts.Envelope[APIResult]{
				OK:     true,
				Source: source,
				Data:   apiResultWithOptions(entry, options.IncludeSafety),
			})
		}
	}
	return withRepoDiagnostics(repo, contracts.Envelope[APIResult]{OK: true, Source: source, Data: APIResult{Name: options.Name}})
}

func lookupAPIEntry(idx *analyze.Index, options LookupAPIOptions) (analyze.APIEntry, bool) {
	if entry, ok := idx.APIs[options.Name]; ok {
		return entry, true
	}
	if options.Exact {
		return analyze.APIEntry{}, false
	}
	for _, entry := range idx.APIs {
		if containsFold(entry.Name, options.Name) {
			return entry, true
		}
	}
	return analyze.APIEntry{}, false
}

func SearchBlizzardAPI(repo analyze.Repository, idx *analyze.Index, options APISearchOptions) contracts.Envelope[APISearchData] {
	source := sourceFor(repo)
	if !repo.Capabilities.APIDocumentation {
		env := capabilityUnavailable[APISearchData](source, "API documentation is unavailable for this source")
		env.Data.Results = []APIResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[APISearchData]{OK: true, Source: source, Data: APISearchData{Results: []APIResult{}}}
	if idx == nil {
		return withRepoDiagnostics(repo, env)
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	for _, entry := range idx.APIs {
		if !matchesAPISearch(entry, options) {
			continue
		}
		env.Data.Results = append(env.Data.Results, apiResult(entry))
		if len(env.Data.Results) >= limit {
			break
		}
	}
	return withRepoDiagnostics(repo, env)
}

func matchesAPISearch(entry analyze.APIEntry, options APISearchOptions) bool {
	if options.Query != "" && !matchesAPIQuery(entry, options.Query) {
		return false
	}
	if !matchesAPIType(entry, options.Type) {
		return false
	}
	if options.Safety != "" && !strings.EqualFold(string(analyze.ClassifySafety(entry.Safety).Level), options.Safety) {
		return false
	}
	if options.IncludeUnsafeOnly {
		level := analyze.ClassifySafety(entry.Safety).Level
		if level == analyze.RiskSafe || level == analyze.RiskNeverSecret {
			return false
		}
	}
	return true
}

func matchesAPIQuery(entry analyze.APIEntry, query string) bool {
	namespace, system := apiNamespaceAndSystem(entry)
	if containsFold(entry.Name, query) || containsFold(entry.Type, query) || containsFold(entry.Signature, query) || containsFold(entry.Path, query) || containsFold(namespace, query) || containsFold(system, query) {
		return true
	}
	for _, params := range [][]analyze.APIParam{entry.Arguments, entry.Returns, entry.Fields} {
		for _, param := range params {
			if containsFold(param.Name, query) || containsFold(param.Type, query) || containsFold(param.Mixin, query) || containsFold(param.Default, query) {
				return true
			}
		}
	}
	for _, value := range entry.Values {
		if containsFold(value.Name, query) || containsFold(value.Type, query) {
			return true
		}
		if value.Value != nil && containsFold(strconv.Itoa(*value.Value), query) {
			return true
		}
	}
	return false
}

func matchesAPIType(entry analyze.APIEntry, requested string) bool {
	if requested == "" || strings.EqualFold(requested, "all") {
		return true
	}
	if strings.EqualFold(entry.Type, requested) {
		return true
	}
	if strings.EqualFold(requested, "function") {
		return entry.Type == "Function"
	}
	if strings.EqualFold(requested, "event") {
		return entry.Type == "Event"
	}
	if strings.EqualFold(requested, "table") {
		return entry.Type == "Table" || entry.Type == "Structure" || entry.Type == "CallbackType" || entry.Type == "Enumeration" || entry.Type == "Constants" || entry.Type == "Constant"
	}
	return false
}

func GetAPINamespace(repo analyze.Repository, idx *analyze.Index, namespace string) contracts.Envelope[APISearchData] {
	if namespace == "list" {
		source := sourceFor(repo)
		if !repo.Capabilities.APIDocumentation {
			env := capabilityUnavailable[APISearchData](source, "API documentation is unavailable for this source")
			env.Data.Results = []APIResult{}
			return withRepoDiagnostics(repo, env)
		}
		env := contracts.Envelope[APISearchData]{OK: true, Source: source, Data: APISearchData{Results: []APIResult{}}}
		if idx == nil {
			return withRepoDiagnostics(repo, env)
		}
		summaries := map[string]NamespaceResult{}
		systems := map[string]bool{}
		for _, entry := range idx.APIs {
			if entry.Type == "System" {
				env.Data.Results = append(env.Data.Results, apiResult(entry))
				ns, system := apiNamespaceAndSystem(entry)
				if ns == "" {
					ns = entry.Name
				}
				summary := summaries[ns]
				summary.Namespace = ns
				if system != "" && !systems[ns+"\x00"+system] {
					systems[ns+"\x00"+system] = true
					summary.SystemCount++
				}
				summaries[ns] = summary
			}
		}
		for _, entry := range idx.APIs {
			ns, _ := apiNamespaceAndSystem(entry)
			if ns == "" || entry.Type != "Function" {
				continue
			}
			summary := summaries[ns]
			summary.Namespace = ns
			summary.FunctionCount++
			summaries[ns] = summary
		}
		env.Data.Namespaces = namespaceSummaries(summaries)
		return withRepoDiagnostics(repo, env)
	}
	source := sourceFor(repo)
	if !repo.Capabilities.APIDocumentation {
		env := capabilityUnavailable[APISearchData](source, "API documentation is unavailable for this source")
		env.Data.Results = []APIResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[APISearchData]{OK: true, Source: source, Data: APISearchData{Results: []APIResult{}}}
	if idx == nil {
		return withRepoDiagnostics(repo, env)
	}
	env.Data.Namespace = namespace
	systemSet := map[string]bool{}
	for _, entry := range idx.APIs {
		if !entryInNamespace(entry, namespace) {
			continue
		}
		result := apiResult(entry)
		env.Data.Results = append(env.Data.Results, result)
		switch entry.Type {
		case "Function":
			env.Data.Functions = append(env.Data.Functions, result)
		case "Event":
			env.Data.Events = append(env.Data.Events, result)
		case "System":
			if result.System != "" && !systemSet[result.System] {
				systemSet[result.System] = true
				env.Data.Systems = append(env.Data.Systems, result.System)
			}
		default:
			env.Data.Tables = append(env.Data.Tables, result)
		}
	}
	sort.Strings(env.Data.Systems)
	return withRepoDiagnostics(repo, env)
}

func SearchFrameXML(repo analyze.Repository, idx *analyze.Index, options FrameXMLSearchOptions) contracts.Envelope[FrameXMLSearchData] {
	source := sourceFor(repo)
	if !repo.Capabilities.FrameXML {
		env := capabilityUnavailable[FrameXMLSearchData](source, "FrameXML is unavailable for this source")
		env.Data.Results = []analyze.SearchResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[FrameXMLSearchData]{OK: true, Source: source, Data: FrameXMLSearchData{Results: []analyze.SearchResult{}}}
	if idx != nil {
		if options.ContextLines == 0 {
			options.ContextLines = 3
		}
		if options.MaxResults == 0 {
			options.MaxResults = 15
		}
		results := idx.SearchFrameXML(analyze.FrameXMLSearchOptions{
			Query:        options.Query,
			FilePattern:  options.FilePattern,
			ContextLines: options.ContextLines,
			MaxResults:   options.MaxResults,
		})
		if results != nil {
			env.Data.Results = results
		}
	}
	return withRepoDiagnostics(repo, env)
}

func FindMixinTemplate(repo analyze.Repository, idx *analyze.Index, name, kind string, limit int) contracts.Envelope[MixinTemplateData] {
	source := sourceFor(repo)
	if !repo.Capabilities.Mixins {
		env := capabilityUnavailable[MixinTemplateData](source, "mixins/templates are unavailable for this source")
		env.Data.Results = []MixinTemplateResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[MixinTemplateData]{OK: true, Source: source, Data: MixinTemplateData{Results: []MixinTemplateResult{}}}
	if idx == nil || limit <= 0 {
		return withRepoDiagnostics(repo, env)
	}
	for _, entry := range idx.FrameXML {
		if containsFold(entry.Name, name) && matchesMixinKind(entry.Kind, kind) {
			env.Data.Results = append(env.Data.Results, MixinTemplateResult{
				Name:     entry.Name,
				Path:     entry.Path,
				Line:     entry.Line,
				Kind:     entry.Kind,
				Inherits: entry.Inherits,
				Snippet:  entry.Snippet,
			})
			if len(env.Data.Results) == limit {
				break
			}
		}
	}
	return withRepoDiagnostics(repo, env)
}

func matchesMixinKind(entryKind, requested string) bool {
	if requested == "" || strings.EqualFold(requested, "all") {
		return true
	}
	return strings.EqualFold(entryKind, requested)
}

func GetWowConstants(repo analyze.Repository, idx *analyze.Index, options ConstantsOptions) contracts.Envelope[ConstantsData] {
	source := sourceFor(repo)
	if !repo.Capabilities.Constants {
		env := capabilityUnavailable[ConstantsData](source, "constants are unavailable for this source")
		env.Data.Results = []ConstantResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[ConstantsData]{OK: true, Source: source, Data: ConstantsData{Results: []ConstantResult{}}}
	if idx == nil {
		return withRepoDiagnostics(repo, env)
	}
	for _, entry := range idx.Constants {
		if options.Name != "list" && !containsFold(entry.Name, options.Name) {
			continue
		}
		if options.Filter != "" && !containsFold(entry.Name, options.Filter) && !containsFold(entry.Type, options.Filter) && !containsFold(entry.Path, options.Filter) {
			continue
		}
		if options.Kind != "" && !strings.EqualFold(entry.Type, options.Kind) {
			continue
		}
		env.Data.Results = append(env.Data.Results, constantResult(entry))
		if options.Limit > 0 && len(env.Data.Results) >= options.Limit {
			break
		}
	}
	return withRepoDiagnostics(repo, env)
}

func ExplainAPISafety(repo analyze.Repository, idx *analyze.Index, symbol, scenario string) contracts.Envelope[SafetyExplanationData] {
	source := sourceFor(repo)
	if scenario == "" {
		scenario = "default"
	}
	meta := analyze.SafetyMetadata{}
	if idx != nil {
		if entry, ok := idx.APIs[symbol]; ok {
			meta = entry.Safety
		}
	}
	stableMeta := stableSafetyMetadata(meta)
	return withRepoDiagnostics(repo, contracts.Envelope[SafetyExplanationData]{
		OK:     true,
		Source: source,
		Data: SafetyExplanationData{
			Raw:            stableMeta,
			Classification: stableSafetyClassification(analyze.ClassifySafety(meta)),
			Explanation:    analyze.ExplainSafety(meta, scenario),
		},
	})
}

func GetAPIEvents(repo analyze.Repository, idx *analyze.Index, options EventOptions) contracts.Envelope[EventData] {
	source := sourceFor(repo)
	if !repo.Capabilities.APIDocumentation {
		env := capabilityUnavailable[EventData](source, "API documentation is unavailable for this source")
		env.Data.Results = []APIResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[EventData]{OK: true, Source: source, Data: EventData{Results: []APIResult{}}}
	if idx == nil {
		return withRepoDiagnostics(repo, env)
	}
	for _, entry := range idx.APIs {
		if entry.Type == "Event" && (options.Event == "list" || containsFold(entry.Name, options.Event)) && matchesEventFilter(entry, options.Filter) {
			env.Data.Results = append(env.Data.Results, apiResult(entry))
		}
	}
	return withRepoDiagnostics(repo, env)
}

func matchesEventFilter(entry analyze.APIEntry, filter string) bool {
	if filter == "" {
		return true
	}
	if containsFold(entry.Name, filter) || containsFold(entry.Signature, filter) {
		return true
	}
	for _, arg := range entry.Arguments {
		if containsFold(arg.Name, filter) || containsFold(arg.Type, filter) {
			return true
		}
	}
	return false
}

func CheckAPIDeprecation(repo analyze.Repository, luaCode string) contracts.Envelope[DeprecationData] {
	return CheckAPIDeprecationWithIndex(repo, nil, luaCode)
}

func CheckAPIDeprecationWithIndex(repo analyze.Repository, idx *analyze.Index, luaCode string) contracts.Envelope[DeprecationData] {
	env := contracts.Envelope[DeprecationData]{OK: true, Source: sourceFor(repo), Data: DeprecationData{Deprecated: []string{}, Details: []DeprecationFinding{}, UnknownAPIs: []string{}}}
	for lineIndex, line := range strings.Split(luaCode, "\n") {
		searchLine := stripLuaComment(line)
		for name, migration := range knownMigrations {
			for _, column := range findFunctionCallColumns(searchLine, name) {
				env.Data.Deprecated = append(env.Data.Deprecated, name)
				env.Data.Details = append(env.Data.Details, DeprecationFinding{
					Function:    name,
					Line:        lineIndex + 1,
					Column:      column,
					Replacement: migration.Replacement,
					Patch:       migration.Patch,
					Notes:       migration.Notes,
				})
				env.Data.Warnings = append(env.Data.Warnings, migrationWarning(repo, idx, migration.Replacement)...)
			}
		}
	}
	if len(env.Data.Deprecated) == 0 {
		env.Data.Summary = "No deprecated API calls found."
	} else {
		env.Data.Summary = "Found " + strconv.Itoa(len(env.Data.Deprecated)) + " deprecated API call(s) that should be updated."
	}
	return withRepoDiagnostics(repo, env)
}

func SuggestAPIMigration(repo analyze.Repository, oldFunction string) contracts.Envelope[MigrationData] {
	return SuggestAPIMigrationWithIndex(repo, nil, oldFunction)
}

func SuggestAPIMigrationWithIndex(repo analyze.Repository, idx *analyze.Index, oldFunction string) contracts.Envelope[MigrationData] {
	env := contracts.Envelope[MigrationData]{
		OK:     true,
		Source: sourceFor(repo),
		Data:   MigrationData{OldFunction: oldFunction, Suggestions: []string{}},
	}
	if migration, ok := knownMigrations[oldFunction]; ok {
		env.Data.Replacement = migration.Replacement
		env.Data.Patch = migration.Patch
		env.Data.Notes = migration.Notes
		env.Data.CodeExample = migrationExample(oldFunction, migration)
		env.Data.Suggestions = append(env.Data.Suggestions, migration.Replacement)
		env.Data.Warnings = append(env.Data.Warnings, migrationWarning(repo, idx, migration.Replacement)...)
		return withRepoDiagnostics(repo, env)
	}
	if idx != nil {
		if entry, ok := lookupAPIEntry(idx, LookupAPIOptions{Name: oldFunction}); ok {
			env.Data.Replacement = entry.Name
			env.Data.Patch = "unknown"
			env.Data.Notes = "Found matching function in selected API documentation."
			env.Data.Suggestions = append(env.Data.Suggestions, entry.Name)
			return withRepoDiagnostics(repo, env)
		}
	}
	env.Data.Patch = "unknown"
	env.Data.Warnings = append(env.Data.Warnings, "no built-in migration is known for "+oldFunction)
	return withRepoDiagnostics(repo, env)
}

func findFunctionCallColumns(line, name string) []int {
	columns := []int{}
	start := 0
	for {
		idx := strings.Index(line[start:], name)
		if idx < 0 {
			return columns
		}
		idx += start
		after := idx + len(name)
		if hasCallBoundary(line, idx, after) {
			rest := strings.TrimLeft(line[after:], " \t")
			if strings.HasPrefix(rest, "(") {
				columns = append(columns, idx+1)
			}
		}
		start = after
	}
}

func hasCallBoundary(line string, start, end int) bool {
	if start > 0 {
		prev := line[start-1]
		if isIdentifierByte(prev) {
			return false
		}
	}
	if end < len(line) {
		next := line[end]
		if isIdentifierByte(next) {
			return false
		}
	}
	return true
}

func isIdentifierByte(ch byte) bool {
	return ch == '_' || ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func stripLuaComment(line string) string {
	inSingle := false
	inDouble := false
	for i := 0; i < len(line)-1; i++ {
		ch := line[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '-':
			if !inSingle && !inDouble && line[i+1] == '-' {
				return line[:i]
			}
		}
	}
	return line
}

func migrationExample(oldFunction string, migration knownMigration) string {
	lines := []string{
		"-- OLD (deprecated):",
		"-- local result = " + oldFunction + "(args)",
		"",
		"-- NEW:",
		"-- local result = " + migration.Replacement + "(args)",
	}
	if strings.Contains(migration.Notes, "Returns a table") {
		lines = append(lines,
			"-- Note: The new API returns a table instead of multiple values.",
			"-- Access fields via result.fieldName instead of positional returns.",
		)
	}
	return strings.Join(lines, "\n")
}

func migrationWarning(repo analyze.Repository, idx *analyze.Index, replacement string) []string {
	warnings := []string{"Use " + replacement + " when available."}
	if isClassicLikeClient(repo.Alias) {
		warnings = append(warnings, "verify replacement availability on Classic before using it")
	}
	if idx != nil {
		if _, ok := idx.APIs[replacement]; !ok {
			warnings = append(warnings, replacement+" is not documented in the selected source; verify before using it")
		}
	}
	return warnings
}

func isClassicLikeClient(alias string) bool {
	return strings.Contains(strings.ToLower(alias), "classic")
}

func GetWidgetAPI(repo analyze.Repository, idx *analyze.Index, widgetType string) contracts.Envelope[WidgetAPIData] {
	source := sourceFor(repo)
	if !repo.Capabilities.WidgetDocs {
		env := capabilityUnavailable[WidgetAPIData](source, "widget documentation is unavailable for this source")
		env.Data.WidgetType = widgetType
		env.Data.Results = []APIResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[WidgetAPIData]{OK: true, Source: source, Data: WidgetAPIData{WidgetType: widgetType, Results: []APIResult{}}}
	if idx == nil {
		return withRepoDiagnostics(repo, env)
	}
	if widgetType == "list" {
		for name := range idx.Widgets {
			env.Data.Results = append(env.Data.Results, APIResult{Name: name, Type: "Widget"})
		}
		return withRepoDiagnostics(repo, env)
	}
	for _, entry := range idx.Widgets[widgetType] {
		env.Data.Results = append(env.Data.Results, apiResult(entry))
	}
	if len(env.Data.Results) == 0 {
		for name, methods := range idx.Widgets {
			if containsFold(name, widgetType) {
				env.Data.Results = append(env.Data.Results, APIResult{Name: name, Type: "Widget"})
				for _, method := range methods {
					env.Data.Results = append(env.Data.Results, apiResult(method))
				}
			}
		}
	}
	return withRepoDiagnostics(repo, env)
}

func LookupCVar(repo analyze.Repository, idx *analyze.Index, options CVarLookupOptions) contracts.Envelope[CVarData] {
	source := sourceFor(repo)
	if !repo.Capabilities.CVars {
		env := capabilityUnavailable[CVarData](source, "CVar references are unavailable for this source")
		env.Data.Results = []CVarResult{}
		return withRepoDiagnostics(repo, env)
	}
	env := contracts.Envelope[CVarData]{OK: true, Source: source, Data: CVarData{Results: []CVarResult{}}}
	if idx == nil {
		return withRepoDiagnostics(repo, env)
	}
	names := make([]string, 0, len(idx.CVars))
	for name := range idx.CVars {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := idx.CVars[names[i]]
		right := idx.CVars[names[j]]
		if len(left.References) != len(right.References) {
			return len(left.References) > len(right.References)
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	if options.Name == "list" {
		for _, name := range names {
			env.Data.Results = append(env.Data.Results, cvarResults(idx.CVars[name], options.Detail)...)
		}
		return withRepoDiagnostics(repo, env)
	}
	for _, name := range names {
		entry := idx.CVars[name]
		if containsFold(entry.Name, options.Name) {
			env.Data.Results = append(env.Data.Results, cvarResults(entry, options.Detail)...)
		}
	}
	return withRepoDiagnostics(repo, env)
}

func ValidateTOC(tocContent, tocPath, addonName string) contracts.Envelope[TOCValidationData] {
	return ValidateTOCWithOptions(tocContent, tocPath, addonName, TOCValidationOptions{})
}

func ValidateTOCWithOptions(tocContent, tocPath, addonName string, options TOCValidationOptions) contracts.Envelope[TOCValidationData] {
	env := contracts.Envelope[TOCValidationData]{OK: true, Data: TOCValidationData{
		AddonName: addonName,
		Parsed:    TOCParsed{InterfaceVersions: []string{}, Files: []string{}, Metadata: map[string]string{}},
	}}
	if strings.TrimSpace(tocContent) == "" && strings.TrimSpace(tocPath) == "" {
		env.OK = false
		env.Data.Errors = append(env.Data.Errors, "tocContent or tocPath is required")
		env.Data.Valid = false
		return env
	}
	if strings.TrimSpace(tocContent) == "" && strings.TrimSpace(tocPath) != "" {
		b, err := os.ReadFile(tocPath)
		if err != nil {
			env.OK = false
			env.Data.Errors = append(env.Data.Errors, err.Error())
			env.Data.Valid = false
			return env
		}
		tocContent = string(b)
	}
	env.Data.Parsed = parseTOC(tocContent)
	if _, ok := env.Data.Parsed.Metadata["Interface"]; !ok {
		env.Data.Errors = append(env.Data.Errors, "Missing required field: ## Interface")
	}
	if _, ok := env.Data.Parsed.Metadata["Title"]; !ok {
		env.Data.Errors = append(env.Data.Errors, "Missing required field: ## Title")
	}
	for _, version := range env.Data.Parsed.InterfaceVersions {
		if !validInterfaceFormat(version) {
			env.Data.Errors = append(env.Data.Errors, "Invalid Interface version format: '"+version+"'")
		} else if label, ok := knownInterfaceVersions[version]; ok {
			env.Data.Info = append(env.Data.Info, "Interface "+version+" = "+label)
		} else if closest := closestInterfaceVersion(version); closest != "" {
			env.Data.Warnings = append(env.Data.Warnings, "Unknown Interface version: "+version+"; did you mean "+closest+" ("+knownInterfaceVersions[closest]+")?")
		} else {
			env.Data.Warnings = append(env.Data.Warnings, "Unknown Interface version: "+version)
		}
	}
	if len(env.Data.Parsed.InterfaceVersions) == 1 {
		env.Data.Info = append(env.Data.Info, "Single Interface version; consider adding multiple versions for wider compatibility")
	}
	if env.Data.Parsed.Title != "" && addonName != "" && !strings.Contains(env.Data.Parsed.Title, addonName) {
		env.Data.Warnings = append(env.Data.Warnings, "Title '"+env.Data.Parsed.Title+"' doesn't contain expected addon name '"+addonName+"'")
	}
	if svName := env.Data.Parsed.Metadata["SavedVariables"]; svName != "" && !strings.HasSuffix(svName, "DB") && !strings.HasSuffix(svName, "Settings") && !strings.HasSuffix(svName, "Data") {
		env.Data.Info = append(env.Data.Info, "SavedVariables '"+svName+"'; convention is to suffix with DB, Settings, or Data")
	}
	if len(env.Data.Parsed.Files) == 0 {
		env.Data.Errors = append(env.Data.Errors, "No Lua/XML files referenced in TOC")
	} else if !hasMainLua(env.Data.Parsed.Files) {
		env.Data.Info = append(env.Data.Info, "No 'main.lua' file found in TOC; ensure your entry point is listed")
	}
	for _, file := range env.Data.Parsed.Files {
		if strings.Contains(file, "\\") {
			env.Data.Warnings = append(env.Data.Warnings, "Backslash in file path; use forward slashes: "+strings.ReplaceAll(file, "\\", "/"))
		}
	}
	if _, ok := env.Data.Parsed.Metadata["Category-enUS"]; !ok && env.Data.Parsed.Title != "" {
		env.Data.Info = append(env.Data.Info, "Consider adding ## Category-enUS for addon browser categorization")
	}
	if options.SourceVersion != "" {
		iface := tocInterfaceValue(tocContent)
		expected := interfaceValueForVersion(options.SourceVersion)
		if iface != "" && expected != "" && iface != expected {
			env.Data.Warnings = append(env.Data.Warnings, "Interface "+iface+" may not match source version "+options.SourceVersion+" (expected "+expected+")")
		}
	}
	env.Data.Valid = len(env.Data.Errors) == 0
	return env
}

func tocInterfaceValue(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## Interface:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "## Interface:"))
		}
	}
	return ""
}

func parseTOC(content string) TOCParsed {
	parsed := TOCParsed{InterfaceVersions: []string{}, Files: []string{}, Metadata: map[string]string{}}
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "##") {
			keyValue := strings.TrimSpace(strings.TrimPrefix(line, "##"))
			key, value, ok := strings.Cut(keyValue, ":")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			parsed.Metadata[key] = value
			switch key {
			case "Interface":
				parsed.InterfaceVersions = splitMetadataList(value)
			case "Title":
				parsed.Title = value
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		parsed.Files = append(parsed.Files, line)
	}
	parsed.SavedVariables = splitMetadataList(parsed.Metadata["SavedVariables"])
	parsed.SavedVariablesPerCharacter = splitMetadataList(parsed.Metadata["SavedVariablesPerCharacter"])
	parsed.Dependencies = splitMetadataList(parsed.Metadata["Dependencies"])
	parsed.OptionalDeps = splitMetadataList(parsed.Metadata["OptionalDeps"])
	return parsed
}

func splitMetadataList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func validInterfaceFormat(version string) bool {
	if len(version) < 5 || len(version) > 6 {
		return false
	}
	for _, ch := range version {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func closestInterfaceVersion(version string) string {
	num, err := strconv.Atoi(version)
	if err != nil {
		return ""
	}
	closest := ""
	minDiff := int(^uint(0) >> 1)
	for known := range knownInterfaceVersions {
		knownNum, err := strconv.Atoi(known)
		if err != nil {
			continue
		}
		diff := knownNum - num
		if diff < 0 {
			diff = -diff
		}
		if diff < minDiff {
			minDiff = diff
			closest = known
		}
	}
	if minDiff < 500 {
		return closest
	}
	return ""
}

func hasMainLua(files []string) bool {
	for _, file := range files {
		if strings.EqualFold(file, "main.lua") {
			return true
		}
	}
	return false
}

func interfaceValueForVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return ""
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ""
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return ""
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return ""
	}
	return strconv.Itoa(major*10000 + minor*100 + patch)
}

func sourceFor(repo analyze.Repository) contracts.SourceTransparency {
	return contracts.SourceTransparency{
		Client:       repo.Alias,
		RequestedRef: repo.RequestedRef,
		ResolvedRef:  repo.ResolvedRef,
		Version:      repo.Version,
		Path:         repo.Path,
	}
}

func apiResult(entry analyze.APIEntry) APIResult {
	return apiResultWithOptions(entry, true)
}

func apiResultWithOptions(entry analyze.APIEntry, includeSafety bool) APIResult {
	namespace, system := apiNamespaceAndSystem(entry)
	result := APIResult{
		Name:      entry.Name,
		Namespace: namespace,
		System:    system,
		Type:      entry.Type,
		Path:      entry.Path,
		Signature: entry.Signature,
		Arguments: stableAPIParams(entry.Arguments),
		Returns:   stableAPIParams(entry.Returns),
		Fields:    entry.Fields,
		Values:    entry.Values,
	}
	if includeSafety {
		result.Safety = &APISafetyResult{
			Raw:            stableSafetyMetadata(entry.Safety),
			Classification: stableSafetyClassification(analyze.ClassifySafety(entry.Safety)),
		}
	}
	return result
}

func stableAPIParams(params []analyze.APIParam) []analyze.APIParam {
	if params == nil {
		return []analyze.APIParam{}
	}
	return params
}

func stableSafetyMetadata(meta analyze.SafetyMetadata) analyze.SafetyMetadata {
	if meta.SecretArgumentsAddAspect == nil {
		meta.SecretArgumentsAddAspect = []string{}
	}
	if meta.SecretReturnsForAspect == nil {
		meta.SecretReturnsForAspect = []string{}
	}
	if meta.RestrictedTypes == nil {
		meta.RestrictedTypes = []string{}
	}
	if meta.Fields == nil {
		meta.Fields = []analyze.SafetyField{}
	}
	return meta
}

func stableSafetyClassification(classification analyze.SafetyClassification) analyze.SafetyClassification {
	if classification.Fields == nil {
		classification.Fields = []analyze.SafetyField{}
	}
	return classification
}

func apiNamespaceAndSystem(entry analyze.APIEntry) (string, string) {
	if entry.Namespace != "" || entry.System != "" {
		return entry.Namespace, entry.System
	}
	if entry.Type == "System" {
		return entry.Name, entry.Name
	}
	if before, _, ok := strings.Cut(entry.Name, "."); ok {
		return before, before
	}
	return "", ""
}

func entryInNamespace(entry analyze.APIEntry, namespace string) bool {
	ns, system := apiNamespaceAndSystem(entry)
	if strings.EqualFold(ns, namespace) || strings.EqualFold(system, namespace) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(entry.Name), strings.ToLower(namespace)+".")
}

func namespaceSummaries(items map[string]NamespaceResult) []NamespaceResult {
	out := make([]NamespaceResult, 0, len(items))
	for _, item := range items {
		if item.Namespace != "" {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Namespace < out[j].Namespace
	})
	return out
}

func constantResult(entry analyze.APIEntry) ConstantResult {
	return ConstantResult{
		Name:   entry.Name,
		Type:   entry.Type,
		Path:   entry.Path,
		System: entry.System,
		Fields: entry.Fields,
		Values: entry.Values,
	}
}

func cvarResults(entry analyze.CVarEntry, detail bool) []CVarResult {
	if detail {
		results := []CVarResult{}
		for _, ref := range entry.References {
			results = append(results, CVarResult{
				Name:         ref.Name,
				DefaultValue: ref.DefaultValue,
				Description:  ref.Description,
				Path:         ref.Path,
				File:         ref.Path,
				Line:         ref.Line,
				Usage:        ref.Usage,
				Snippet:      ref.Snippet,
			})
		}
		return results
	}
	files := []string{}
	seen := map[string]bool{}
	for _, ref := range entry.References {
		if !seen[ref.Path] {
			seen[ref.Path] = true
			files = append(files, ref.Path)
		}
	}
	if len(files) > 5 {
		files = files[:5]
	}
	return []CVarResult{{
		Name:         entry.Name,
		DefaultValue: entry.DefaultValue,
		Path:         entry.Path,
		References:   len(entry.References),
		Files:        files,
	}}
}

func capabilityUnavailable[T any](source contracts.SourceTransparency, message string) contracts.Envelope[T] {
	return contracts.Envelope[T]{
		OK:     false,
		Source: source,
		Error:  &contracts.ToolError{Code: contracts.ErrCapabilityUnavailable, Message: message},
	}
}

func withRepoDiagnostics[T any](repo analyze.Repository, env contracts.Envelope[T]) contracts.Envelope[T] {
	env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
	return env
}

func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
