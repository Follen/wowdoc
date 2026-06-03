package mcp

type JSONSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

type SchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func (s JSONSchema) Requires(name string) bool {
	for _, required := range s.Required {
		if required == name {
			return true
		}
	}
	return false
}

func ToolInputSchemas() map[string]JSONSchema {
	schemas := map[string]JSONSchema{}
	for name, primary := range map[string]string{
		"get_api_namespace":     "namespace",
		"check_api_deprecation": "luaCode",
		"suggest_api_migration": "oldFunction",
		"get_widget_api":        "widgetType",
		"explain_api_safety":    "symbol",
	} {
		schemas[name] = sourceSchema(primary)
	}
	schemas["lookup_blizzard_api"] = JSONSchema{Type: "object", Required: []string{"client", "name"}, Properties: map[string]SchemaProperty{
		"client":        {Type: "string", Description: "Required source client alias."},
		"ref":           {Type: "string", Description: "Optional branch, tag, or commit."},
		"name":          {Type: "string"},
		"exact":         {Type: "boolean", Description: "When true, require an exact API name. Defaults to false for fuzzy substring lookup."},
		"includeSafety": {Type: "boolean"},
	}}
	schemas["get_wow_constants"] = JSONSchema{Type: "object", Required: []string{"client", "name"}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref":    {Type: "string", Description: "Optional branch, tag, or commit."},
		"name":   {Type: "string"},
		"filter": {Type: "string"},
		"kind":   {Type: "string"},
		"limit":  {Type: "number"},
	}}
	schemas["lookup_cvar"] = JSONSchema{Type: "object", Required: []string{"client", "name"}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref":    {Type: "string", Description: "Optional branch, tag, or commit."},
		"name":   {Type: "string"},
		"detail": {Type: "boolean"},
	}}
	schemas["search_blizzard_api"] = JSONSchema{Type: "object", Required: []string{"client", "query"}, Properties: map[string]SchemaProperty{
		"client":            {Type: "string", Description: "Required source client alias."},
		"ref":               {Type: "string", Description: "Optional branch, tag, or commit."},
		"query":             {Type: "string"},
		"type":              {Type: "string"},
		"limit":             {Type: "number"},
		"safety":            {Type: "string"},
		"scenario":          {Type: "string"},
		"includeUnsafeOnly": {Type: "boolean"},
	}}
	schemas["search_framexml"] = JSONSchema{Type: "object", Required: []string{"client", "query"}, Properties: map[string]SchemaProperty{
		"client":       {Type: "string", Description: "Required source client alias."},
		"ref":          {Type: "string", Description: "Optional branch, tag, or commit."},
		"query":        {Type: "string"},
		"filePattern":  {Type: "string"},
		"contextLines": {Type: "number"},
		"maxResults":   {Type: "number"},
	}}
	schemas["get_api_events"] = JSONSchema{Type: "object", Required: []string{"client", "event"}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref":    {Type: "string", Description: "Optional branch, tag, or commit."},
		"event":  {Type: "string"},
		"filter": {Type: "string"},
	}}
	schemas["find_mixin_template"] = JSONSchema{Type: "object", Required: []string{"client", "name"}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref":    {Type: "string", Description: "Optional branch, tag, or commit."},
		"name":   {Type: "string"},
		"kind":   {Type: "string"},
		"limit":  {Type: "number"},
	}}
	schemas["list_clients"] = JSONSchema{Type: "object", Properties: map[string]SchemaProperty{
		"includeDiagnostics": {Type: "boolean"},
		"includeRefs":        {Type: "boolean"},
	}}
	schemas["inspect_remote_refs"] = JSONSchema{Type: "object", Properties: map[string]SchemaProperty{
		"client":         {Type: "string", Description: "Optional configured client alias to inspect."},
		"includeVersion": {Type: "boolean", Description: "Resolve the configured source and include detected version."},
	}}
	schemas["validate_toc"] = JSONSchema{Type: "object", Properties: map[string]SchemaProperty{
		"tocContent": {Type: "string"},
		"tocPath":    {Type: "string"},
		"client":     {Type: "string"},
		"ref":        {Type: "string"},
		"addonName":  {Type: "string"},
	}}
	return schemas
}

func sourceSchema(primary string) JSONSchema {
	return JSONSchema{Type: "object", Required: []string{"client", primary}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref":    {Type: "string", Description: "Optional branch, tag, or commit."},
		primary:  {Type: "string"},
	}}
}
