package tools

func DefaultRegistry() Registry {
	names := []string{
		"list_clients",
		"lookup_blizzard_api",
		"search_blizzard_api",
		"get_api_namespace",
		"get_api_events",
		"search_framexml",
		"validate_toc",
		"check_api_deprecation",
		"suggest_api_migration",
		"get_wow_constants",
		"get_widget_api",
		"find_mixin_template",
		"lookup_cvar",
		"explain_api_safety",
		"inspect_remote_refs",
	}
	reg := Registry{Tools: map[string]ToolDefinition{}}
	for _, name := range names {
		reg.Tools[name] = ToolDefinition{Name: name, SourceBacked: name != "validate_toc" && name != "list_clients"}
	}
	return reg
}
