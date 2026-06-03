package analyze

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type APIEntry struct {
	Name      string         `json:"name"`
	Namespace string         `json:"namespace,omitempty"`
	System    string         `json:"system,omitempty"`
	Type      string         `json:"type,omitempty"`
	Path      string         `json:"path,omitempty"`
	Signature string         `json:"signature,omitempty"`
	Arguments []APIParam     `json:"arguments,omitempty"`
	Returns   []APIParam     `json:"returns,omitempty"`
	Fields    []APIParam     `json:"fields,omitempty"`
	Values    []APIValue     `json:"values,omitempty"`
	Safety    SafetyMetadata `json:"safety,omitempty"`
}

type APIParam struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Nilable bool   `json:"nilable,omitempty"`
	Mixin   string `json:"mixin,omitempty"`
	Default string `json:"default,omitempty"`
}

type APIValue struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Value *int   `json:"value,omitempty"`
}

type FrameXMLEntry struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Line     int      `json:"line,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Inherits []string `json:"inherits,omitempty"`
	Snippet  string   `json:"snippet,omitempty"`
}

type SearchResult struct {
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Text   string   `json:"text"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

type CVarEntry struct {
	Name         string          `json:"name"`
	DefaultValue string          `json:"defaultValue,omitempty"`
	Description  string          `json:"description,omitempty"`
	Path         string          `json:"path,omitempty"`
	References   []CVarReference `json:"references,omitempty"`
}

type CVarReference struct {
	Name         string `json:"name"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Description  string `json:"description,omitempty"`
	Path         string `json:"path,omitempty"`
	Line         int    `json:"line,omitempty"`
	Usage        string `json:"usage,omitempty"`
	Snippet      string `json:"snippet,omitempty"`
}

type Index struct {
	APIs      map[string]APIEntry      `json:"apis"`
	Constants map[string]APIEntry      `json:"constants"`
	FrameXML  map[string]FrameXMLEntry `json:"frameXML"`
	Widgets   map[string][]APIEntry    `json:"widgets"`
	CVars     map[string]CVarEntry     `json:"cvars"`
	Lines     []SearchResult           `json:"lines"`
}

const indexWorkerCount = 4

type indexJob struct {
	path string
}

type indexResult struct {
	path string
	idx  *Index
}

var (
	apiConstantPattern          = regexp.MustCompile(`(?s)APIDocumentation:AddDocumentationTable\(\{\s*Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"(Enumeration|Constant|Table)"`)
	apiSystemPattern            = regexp.MustCompile(`Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"System"`)
	apiTablePattern             = regexp.MustCompile(`(?s)Tables\s*=\s*\{(.*?)\n\s*\},\s*\n\s*Functions\s*=`)
	apiTableEntryPattern        = regexp.MustCompile(`Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"(Structure|Table|CallbackType)"`)
	apiFunctionPattern          = regexp.MustCompile(`(?s)Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"Function"\s*,(.*?)\n\s*Arguments\s*=\s*\{(.*?)\n\s*\},\s*\n\s*Returns\s*=\s*\{(.*?)\n\s*\},`)
	apiFunctionNoReturnsPattern = regexp.MustCompile(`(?s)Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"Function"\s*,(.*?)\n\s*Arguments\s*=\s*\{(.*?)\n\s*\},\s*\n\s*\},`)
	apiEventPattern             = regexp.MustCompile(`(?s)Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"Event"\s*,\s*Payload\s*=\s*\{(.*?)\n\s*\},`)
	apiWidgetPattern            = regexp.MustCompile(`Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"Widget"`)
	apiWidgetMethodPattern      = regexp.MustCompile(`(?s)Name\s*=\s*"([^"]+)"\s*,\s*Type\s*=\s*"ScriptObject"\s*,\s*Arguments\s*=\s*\{(.*?)\n\s*\},`)
	apiParamBlockPattern        = regexp.MustCompile(`(?s)\{(.*?)\}`)
	cvarDocumentedPattern       = regexp.MustCompile(`C_CVar\.RegisterCVar\("([^"]+)",\s*"([^"]*)",\s*"([^"]*)"\)`)
	cvarRegisterPattern         = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.:])RegisterCVar\s*\(\s*["']([^"']+)["'](?:\s*,\s*([^,)]+))?`)
	cvarSetPattern              = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.:])SetCVar\s*\(\s*["']([^"']+)["'](?:\s*,\s*([^,)]+))?`)
	cvarGetPattern              = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.:])GetCVar(?:Bool|Default)?\s*\(\s*["']([^"']+)["']`)
	cvarNamespacePattern        = regexp.MustCompile(`\bC_CVar\.(?:Get|Set|Register)\w*\s*\(\s*["']([^"']+)["'](?:\s*,\s*([^,)]+))?`)
	mixinPattern                = regexp.MustCompile(`^\s*(?:local\s+)?([A-Za-z_][A-Za-z0-9_]*Mixin)\s*=\s*(?:\{|CreateFromMixins\s*\(([^)]*)\))`)
	frameXMLSymbol              = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*(?:Template|Mixin))\s*=`)
	xmlTemplateWithInherit      = regexp.MustCompile(`<\s*(Frame|Button|EditBox|CheckButton|Slider|ScrollFrame|StatusBar|Texture|FontString)\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*\binherits\s*=\s*["']([^"']+)["']`)
	xmlVirtualTemplate          = regexp.MustCompile(`<\s*(Frame|Button|EditBox|CheckButton|Slider|ScrollFrame|StatusBar)\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*\bvirtual\s*=\s*["']true["']`)
)

func BuildIndex(repo Repository) (*Index, error) {
	idx := newIndex()
	if !repo.Valid {
		return idx, nil
	}
	paths := []string{}
	err := filepath.WalkDir(repo.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".lua" && ext != ".xml" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	partials := indexFiles(paths)
	for _, path := range paths {
		mergeIndex(idx, partials[path])
	}
	return idx, nil
}

func newIndex() *Index {
	return &Index{
		APIs:      map[string]APIEntry{},
		Constants: map[string]APIEntry{},
		FrameXML:  map[string]FrameXMLEntry{},
		Widgets:   map[string][]APIEntry{},
		CVars:     map[string]CVarEntry{},
	}
}

func indexFiles(paths []string) map[string]*Index {
	jobs := make(chan indexJob)
	results := make(chan indexResult)
	var wg sync.WaitGroup
	for worker := 0; worker < indexWorkerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- indexResult{path: job.path, idx: indexFile(job.path)}
			}
		}()
	}
	go func() {
		for _, path := range paths {
			jobs <- indexJob{path: path}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	partials := map[string]*Index{}
	for result := range results {
		partials[result.path] = result.idx
	}
	return partials
}

func indexFile(path string) *Index {
	idx := newIndex()
	b, readErr := os.ReadFile(path)
	if readErr != nil {
		return idx
	}
	content := string(b)
	indexDocumentedConstants(idx, path, content)
	indexDocumentedSystems(idx, path, content)
	indexDocumentedTables(idx, path, content)
	indexDocumentedFunctions(idx, path, content)
	indexDocumentedEvents(idx, path, content)
	indexDocumentedWidgets(idx, path, content)
	indexCVars(idx, path, content)
	if isAddOnCode(path) {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			idx.Lines = append(idx.Lines, SearchResult{File: path, Line: i + 1, Text: line})
		}
		indexFrameXMLSymbols(idx, path, lines)
	}
	return idx
}

func mergeIndex(dst, src *Index) {
	if src == nil {
		return
	}
	for key, entry := range src.APIs {
		dst.APIs[key] = entry
	}
	for key, entry := range src.Constants {
		dst.Constants[key] = entry
	}
	for key, entry := range src.FrameXML {
		dst.FrameXML[key] = entry
	}
	for key, entries := range src.Widgets {
		dst.Widgets[key] = append(dst.Widgets[key], entries...)
	}
	for key, entry := range src.CVars {
		if existing, ok := dst.CVars[key]; ok {
			if existing.DefaultValue == "" {
				existing.DefaultValue = entry.DefaultValue
			}
			if existing.Description == "" {
				existing.Description = entry.Description
			}
			if existing.Path == "" {
				existing.Path = entry.Path
			}
			existing.References = append(existing.References, entry.References...)
			dst.CVars[key] = existing
			continue
		}
		dst.CVars[key] = entry
	}
	dst.Lines = append(dst.Lines, src.Lines...)
}

func frameXMLKind(name string) string {
	switch {
	case strings.HasSuffix(name, "Mixin"):
		return "Mixin"
	case strings.HasSuffix(name, "Template"):
		return "Template"
	default:
		return ""
	}
}

func isAddOnCode(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/Interface/AddOns/") || strings.Contains(normalized, "Interface/AddOns/")
}

func indexDocumentedEvents(idx *Index, path, content string) {
	system, namespace := documentedSystemAndNamespace(content)
	for _, match := range apiEventPattern.FindAllStringSubmatch(content, -1) {
		name := match[1]
		payload := parseAPIParams(match[2])
		idx.APIs[name] = APIEntry{
			Name:      name,
			Namespace: namespace,
			System:    system,
			Type:      "Event",
			Path:      path,
			Signature: signature(name, payload, nil),
			Arguments: payload,
		}
	}
}

func indexDocumentedConstants(idx *Index, path, content string) {
	for _, block := range documentationTableBlocks(content) {
		name := stringField(block, "Name")
		kind := stringField(block, "Type")
		if name == "" || !isConstantKind(kind) {
			continue
		}
		fieldsBlock := extractLuaBlock(block, "Fields")
		idx.Constants[name] = APIEntry{
			Name:   name,
			Type:   kind,
			Path:   path,
			Fields: parseAPIParams(fieldsBlock),
			Values: parseAPIValues(fieldsBlock),
		}
	}
	for _, match := range apiConstantPattern.FindAllStringSubmatch(content, -1) {
		if _, ok := idx.Constants[match[1]]; ok {
			continue
		}
		idx.Constants[match[1]] = APIEntry{Name: match[1], Type: match[2], Path: path}
	}
}

func indexDocumentedTables(idx *Index, path, content string) {
	system, namespace := documentedSystemAndNamespace(content)
	tablesBlock := extractLuaBlock(content, "Tables")
	if tablesBlock != "" {
		for _, block := range splitLuaEntries(tablesBlock) {
			name := stringField(block, "Name")
			kind := stringField(block, "Type")
			if name == "" || kind == "" {
				continue
			}
			fieldsBlock := extractLuaBlock(block, "Fields")
			entry := APIEntry{
				Name:      name,
				Namespace: namespace,
				System:    system,
				Type:      kind,
				Path:      path,
				Fields:    parseAPIParams(fieldsBlock),
			}
			if kind == "Enumeration" || kind == "Constants" {
				entry.Values = parseAPIValues(fieldsBlock)
				idx.Constants[name] = entry
			}
			idx.APIs[name] = entry
		}
		return
	}
	for _, block := range apiTablePattern.FindAllStringSubmatch(content, -1) {
		for _, match := range apiTableEntryPattern.FindAllStringSubmatch(block[1], -1) {
			name := match[1]
			idx.APIs[name] = APIEntry{Name: name, Namespace: namespace, System: system, Type: match[2], Path: path}
		}
	}
}

func indexDocumentedWidgets(idx *Index, path, content string) {
	widget := ""
	if match := apiWidgetPattern.FindStringSubmatch(content); len(match) == 2 {
		widget = match[1]
	}
	if widget == "" {
		return
	}
	for _, match := range apiWidgetMethodPattern.FindAllStringSubmatch(content, -1) {
		name := widget + "." + match[1]
		args := parseAPIParams(match[2])
		idx.Widgets[widget] = append(idx.Widgets[widget], APIEntry{
			Name:      name,
			Type:      "ScriptObject",
			Path:      path,
			Signature: signature(name, args, nil),
			Arguments: args,
		})
	}
}

func indexCVars(idx *Index, path, content string) {
	for lineNumber, rawLine := range strings.Split(content, "\n") {
		code := stripLuaLineComment(rawLine)
		if strings.TrimSpace(code) == "" {
			continue
		}
		for _, ref := range cvarReferencesForLine(path, lineNumber+1, rawLine, code) {
			addCVarReference(idx, ref)
		}
	}
}

func cvarReferencesForLine(path string, line int, rawLine, code string) []CVarReference {
	refs := []CVarReference{}
	seen := map[string]bool{}
	add := func(name, usage, defaultValue, description string) {
		key := name + "\x00" + usage
		if seen[key] {
			return
		}
		seen[key] = true
		refs = append(refs, CVarReference{
			Name:         name,
			DefaultValue: cleanCVarValue(defaultValue),
			Description:  description,
			Path:         path,
			Line:         line,
			Usage:        usage,
			Snippet:      strings.TrimSpace(rawLine),
		})
	}
	for _, match := range cvarDocumentedPattern.FindAllStringSubmatch(code, -1) {
		add(match[1], "Register", match[2], match[3])
	}
	for _, match := range cvarRegisterPattern.FindAllStringSubmatch(code, -1) {
		add(match[1], "Register", match[2], "")
	}
	for _, match := range cvarSetPattern.FindAllStringSubmatch(code, -1) {
		add(match[1], "Set", match[2], "")
	}
	for _, match := range cvarGetPattern.FindAllStringSubmatch(code, -1) {
		add(match[1], "Get", "", "")
	}
	for _, match := range cvarNamespacePattern.FindAllStringSubmatch(code, -1) {
		if seen[match[1]+"\x00Register"] {
			continue
		}
		add(match[1], "Reference", match[2], "")
	}
	return refs
}

func addCVarReference(idx *Index, ref CVarReference) {
	entry := idx.CVars[ref.Name]
	if entry.Name == "" {
		entry.Name = ref.Name
	}
	if entry.DefaultValue == "" {
		entry.DefaultValue = ref.DefaultValue
	}
	if entry.Description == "" {
		entry.Description = ref.Description
	}
	if entry.Path == "" {
		entry.Path = ref.Path
	}
	entry.References = append(entry.References, ref)
	idx.CVars[ref.Name] = entry
}

func cleanCVarValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ";")
	return strings.TrimSpace(value)
}

func stripLuaLineComment(line string) string {
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

func indexFrameXMLSymbols(idx *Index, path string, lines []string) {
	for i, line := range lines {
		if match := mixinPattern.FindStringSubmatch(line); len(match) > 0 {
			idx.FrameXML[match[1]] = FrameXMLEntry{
				Name:     match[1],
				Path:     path,
				Line:     i + 1,
				Kind:     "Mixin",
				Inherits: splitCSV(match[2]),
				Snippet:  strings.TrimSpace(line),
			}
			continue
		}
		for _, match := range frameXMLSymbol.FindAllStringSubmatch(line, -1) {
			name := match[1]
			if _, ok := idx.FrameXML[name]; ok {
				continue
			}
			idx.FrameXML[name] = FrameXMLEntry{Name: name, Path: path, Line: i + 1, Kind: frameXMLKind(name), Snippet: strings.TrimSpace(line)}
		}
		if filepath.Ext(path) != ".xml" {
			continue
		}
		if match := xmlTemplateWithInherit.FindStringSubmatch(line); len(match) > 0 {
			idx.FrameXML[match[2]] = FrameXMLEntry{
				Name:     match[2],
				Path:     path,
				Line:     i + 1,
				Kind:     "Template",
				Inherits: splitCSV(match[3]),
				Snippet:  strings.TrimSpace(line),
			}
			continue
		}
		if match := xmlVirtualTemplate.FindStringSubmatch(line); len(match) > 0 {
			idx.FrameXML[match[2]] = FrameXMLEntry{Name: match[2], Path: path, Line: i + 1, Kind: "Template", Snippet: strings.TrimSpace(line)}
		}
	}
}

func splitCSV(value string) []string {
	values := []string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func indexDocumentedSystems(idx *Index, path, content string) {
	for _, match := range apiSystemPattern.FindAllStringSubmatch(content, -1) {
		name := match[1]
		namespace := stringField(content, "Namespace")
		if namespace == "" {
			namespace = name
		}
		idx.APIs[name] = APIEntry{Name: name, Namespace: namespace, System: name, Type: "System", Path: path}
	}
}

func indexDocumentedFunctions(idx *Index, path, content string) {
	system, namespace := documentedSystemAndNamespace(content)
	if system == "" {
		return
	}
	apiPrefix := namespace
	if apiPrefix == "" {
		apiPrefix = system
	}
	for _, match := range apiFunctionPattern.FindAllStringSubmatch(content, -1) {
		name := apiPrefix + "." + match[1]
		args := parseAPIParams(match[3])
		returns := parseAPIParams(match[4])
		idx.APIs[name] = APIEntry{
			Name:      name,
			Namespace: namespace,
			System:    system,
			Type:      "Function",
			Path:      path,
			Signature: signature(name, args, returns),
			Arguments: args,
			Returns:   returns,
			Safety:    parseSafetyMetadata(match[2], match[4]),
		}
	}
	for _, match := range apiFunctionNoReturnsPattern.FindAllStringSubmatch(content, -1) {
		name := apiPrefix + "." + match[1]
		if _, ok := idx.APIs[name]; ok {
			continue
		}
		args := parseAPIParams(match[3])
		idx.APIs[name] = APIEntry{
			Name:      name,
			Namespace: namespace,
			System:    system,
			Type:      "Function",
			Path:      path,
			Signature: signature(name, args, nil),
			Arguments: args,
			Safety:    parseSafetyMetadata(match[2]),
		}
	}
}

func parseSafetyMetadata(content string, returnBlocks ...string) SafetyMetadata {
	fields := []SafetyField{}
	for _, block := range returnBlocks {
		fields = append(fields, parseSafetyFields(block)...)
	}
	return SafetyMetadata{
		IsProtectedFunction:                       boolField(content, "IsProtectedFunction"),
		SecretArguments:                           stringField(content, "SecretArguments"),
		SecretArgumentsAddAspect:                  stringListField(content, "SecretArgumentsAddAspect"),
		SecretReturnsForAspect:                    stringListField(content, "SecretReturnsForAspect"),
		SecretWhenUnitSpellCastRestricted:         boolField(content, "SecretWhenUnitSpellCastRestricted"),
		SecretWhenCooldownsRestricted:             boolField(content, "SecretWhenCooldownsRestricted"),
		SecretInChatMessagingLockdown:             boolField(content, "SecretInChatMessagingLockdown"),
		RequiresNonSecretAura:                     boolField(content, "RequiresNonSecretAura"),
		RequiresRestrictedAbbreviationBreakpoints: boolField(content, "RequiresRestrictedAbbreviationBreakpoints"),
		ConditionalSecret:                         boolField(content, "ConditionalSecret"),
		IsForbidden:                               boolField(content, "IsForbidden"),
		SetForbidden:                              boolField(content, "SetForbidden"),
		ConstSecretAccessor:                       boolField(content, "ConstSecretAccessor"),
		IsPreventingSecretValues:                  boolField(content, "IsPreventingSecretValues"),
		ReturnsNeverSecret:                        boolField(content, "ReturnsNeverSecret"),
		NeverSecret:                               boolField(content, "NeverSecret"),
		SecretWrapperConstant:                     stringField(content, "SecretWrapperConstant"),
		RestrictedTypes:                           stringListField(content, "RestrictedTypes"),
		Fields:                                    fields,
	}
}

func parseAPIParams(content string) []APIParam {
	params := []APIParam{}
	for _, block := range splitLuaEntries(content) {
		name := stringField(block, "Name")
		if name == "" {
			continue
		}
		params = append(params, APIParam{
			Name:    name,
			Type:    stringField(block, "Type"),
			Nilable: boolValueField(block, "Nilable"),
			Mixin:   stringField(block, "Mixin"),
			Default: stringField(block, "Default"),
		})
	}
	return params
}

func parseAPIValues(content string) []APIValue {
	values := []APIValue{}
	for _, block := range splitLuaEntries(content) {
		name := stringField(block, "Name")
		if name == "" {
			continue
		}
		values = append(values, APIValue{
			Name:  name,
			Type:  stringField(block, "Type"),
			Value: intField(block, "EnumValue"),
		})
	}
	return values
}

func documentedSystemAndNamespace(content string) (string, string) {
	system := ""
	if match := apiSystemPattern.FindStringSubmatch(content); len(match) == 2 {
		system = match[1]
	}
	namespace := stringField(content, "Namespace")
	if namespace == "" {
		namespace = system
	}
	return system, namespace
}

func isConstantKind(kind string) bool {
	return kind == "Enumeration" || kind == "Constant" || kind == "Constants" || kind == "Table"
}

func documentationTableBlocks(content string) []string {
	blocks := []string{}
	start := 0
	for {
		idx := strings.Index(content[start:], "APIDocumentation:AddDocumentationTable(")
		if idx < 0 {
			return blocks
		}
		open := strings.Index(content[start+idx:], "{")
		if open < 0 {
			return blocks
		}
		blockStart := start + idx + open
		block, end := balancedLuaBlock(content, blockStart)
		if block == "" {
			start = blockStart + 1
			continue
		}
		blocks = append(blocks, block)
		start = end
	}
}

func extractLuaBlock(content, blockName string) string {
	pattern := regexp.MustCompile(blockName + `\s*=\s*\{`)
	match := pattern.FindStringIndex(content)
	if len(match) != 2 {
		return ""
	}
	block, _ := balancedLuaBlock(content, match[1]-1)
	if len(block) < 2 {
		return ""
	}
	return block[1 : len(block)-1]
}

func balancedLuaBlock(content string, open int) (string, int) {
	if open < 0 || open >= len(content) || content[open] != '{' {
		return "", open
	}
	depth := 0
	inSingle := false
	inDouble := false
	for i := open; i < len(content); i++ {
		ch := content[i]
		if ch == '\\' && (inSingle || inDouble) {
			i++
			continue
		}
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '{':
			if !inSingle && !inDouble {
				depth++
			}
		case '}':
			if !inSingle && !inDouble {
				depth--
				if depth == 0 {
					return content[open : i+1], i + 1
				}
			}
		}
	}
	return "", open
}

func splitLuaEntries(content string) []string {
	entries := []string{}
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		block, end := balancedLuaBlock(content, i)
		if block == "" {
			continue
		}
		entries = append(entries, block)
		i = end - 1
	}
	return entries
}

func boolField(content, name string) bool {
	pattern := regexp.MustCompile(name + `\s*=\s*true`)
	return pattern.MatchString(content)
}

func boolValueField(content, name string) bool {
	pattern := regexp.MustCompile(name + `\s*=\s*(true|false)`)
	match := pattern.FindStringSubmatch(content)
	return len(match) == 2 && match[1] == "true"
}

func stringField(content, name string) string {
	pattern := regexp.MustCompile(name + `\s*=\s*"([^"]+)"`)
	match := pattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func intField(content, name string) *int {
	pattern := regexp.MustCompile(name + `\s*=\s*(-?\d+)`)
	match := pattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return nil
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}
	return &value
}

func stringListField(content, name string) []string {
	pattern := regexp.MustCompile(`(?s)` + name + `\s*=\s*\{(.*?)\}`)
	match := pattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return nil
	}
	stringPattern := regexp.MustCompile(`"([^"]+)"`)
	values := []string{}
	for _, item := range stringPattern.FindAllStringSubmatch(match[1], -1) {
		values = append(values, item[1])
	}
	return values
}

func parseSafetyFields(content string) []SafetyField {
	fields := []SafetyField{}
	for _, match := range apiParamBlockPattern.FindAllStringSubmatch(content, -1) {
		name := stringField(match[1], "Name")
		if name == "" {
			continue
		}
		field := SafetyField{
			Name:              name,
			ConditionalSecret: boolField(match[1], "ConditionalSecret"),
			NeverSecret:       boolField(match[1], "NeverSecret"),
		}
		if field.ConditionalSecret || field.NeverSecret {
			fields = append(fields, field)
		}
	}
	return fields
}

func signature(name string, args, returns []APIParam) string {
	argNames := make([]string, 0, len(args))
	for _, arg := range args {
		argNames = append(argNames, arg.Name)
	}
	returnNames := make([]string, 0, len(returns))
	for _, ret := range returns {
		returnNames = append(returnNames, ret.Name)
	}
	if len(returnNames) == 0 {
		return name + "(" + strings.Join(argNames, ", ") + ")"
	}
	return name + "(" + strings.Join(argNames, ", ") + ") -> " + strings.Join(returnNames, ", ")
}

type FrameXMLSearchOptions struct {
	Query        string
	FilePattern  string
	ContextLines int
	MaxResults   int
}

func (i *Index) SearchFrameXML(options FrameXMLSearchOptions) []SearchResult {
	if options.MaxResults <= 0 {
		return nil
	}
	var out []SearchResult
	query := strings.ToLower(options.Query)
	for idx, line := range i.Lines {
		if query != "" && !strings.Contains(strings.ToLower(line.Text), query) {
			continue
		}
		if options.FilePattern != "" && !matchesFrameXMLFilePattern(line.File, options.FilePattern) {
			continue
		}
		result := line
		if options.ContextLines > 0 {
			result.Before = frameXMLContext(i.Lines, idx, -options.ContextLines)
			result.After = frameXMLContext(i.Lines, idx, options.ContextLines)
		}
		out = append(out, result)
		if len(out) == options.MaxResults {
			break
		}
	}
	return out
}

func matchesFrameXMLFilePattern(path, pattern string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(base, pattern[1:])
	}
	if strings.Contains(pattern, "**") {
		return true
	}
	return strings.Contains(base, pattern) || strings.Contains(path, pattern)
}

func frameXMLContext(lines []SearchResult, idx, count int) []string {
	if count == 0 {
		return nil
	}
	if count < 0 {
		start := idx + count
		if start < 0 {
			start = 0
		}
		out := []string{}
		for i := start; i < idx; i++ {
			if lines[i].File == lines[idx].File {
				out = append(out, lines[i].Text)
			}
		}
		return out
	}
	end := idx + count
	if end >= len(lines) {
		end = len(lines) - 1
	}
	out := []string{}
	for i := idx + 1; i <= end; i++ {
		if lines[i].File == lines[idx].File {
			out = append(out, lines[i].Text)
		}
	}
	return out
}
