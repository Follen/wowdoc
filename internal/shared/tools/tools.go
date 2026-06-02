package tools

type Handler interface{}

type ToolDefinition struct {
	Name         string
	SourceBacked bool
}

type Registry struct {
	Tools map[string]ToolDefinition
}
