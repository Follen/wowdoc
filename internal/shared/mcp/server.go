package mcp

import (
	"sort"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ServerOptions struct {
	Name    string
	Version string
}

type Server struct {
	sdkServer *sdkmcp.Server
	tools     map[string]JSONSchema
}

func NewServer(options ServerOptions) *Server {
	name := options.Name
	if name == "" {
		name = "wowdoc"
	}

	return &Server{
		sdkServer: sdkmcp.NewServer(&sdkmcp.Implementation{Name: name, Version: options.Version}, nil),
		tools:     ToolInputSchemas(),
	}
}

func (s *Server) RegisteredToolNames() []string {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Server) HasTool(name string) bool {
	_, ok := s.tools[name]
	return ok
}
