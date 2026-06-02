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
	for name := range map[string]bool{
		"lookup_blizzard_api":   true,
		"search_blizzard_api":   true,
		"get_api_namespace":     true,
		"get_api_events":        true,
		"search_framexml":       true,
		"check_api_deprecation": true,
		"suggest_api_migration": true,
		"get_wow_constants":     true,
		"get_widget_api":        true,
		"find_mixin_template":   true,
		"lookup_cvar":           true,
		"explain_api_safety":    true,
	} {
		schemas[name] = sourceSchema("name")
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
