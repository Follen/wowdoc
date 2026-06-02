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
		"lookup_blizzard_api":   "name",
		"search_blizzard_api":   "query",
		"get_api_namespace":     "namespace",
		"get_api_events":        "event",
		"search_framexml":       "query",
		"check_api_deprecation": "luaCode",
		"suggest_api_migration": "oldFunction",
		"get_wow_constants":     "name",
		"get_widget_api":        "widgetType",
		"find_mixin_template":   "name",
		"lookup_cvar":           "name",
		"explain_api_safety":    "symbol",
	} {
		schemas[name] = sourceSchema(primary)
	}
	schemas["list_clients"] = JSONSchema{Type: "object", Properties: map[string]SchemaProperty{
		"includeDiagnostics": {Type: "boolean"},
		"includeRefs":        {Type: "boolean"},
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
	return JSONSchema{Type: "object", Required: []string{"client"}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref":    {Type: "string", Description: "Optional branch, tag, or commit."},
		primary:  {Type: "string"},
	}}
}
