package mcp

import (
	"context"
	"sort"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ServerOptions struct {
	Name    string
	Version string
}

type Server struct {
	sdkServer          *sdkmcp.Server
	tools              map[string]JSONSchema
	sdkRegisteredTools int
}

func NewServer(options ServerOptions) *Server {
	name := options.Name
	if name == "" {
		name = "wowdoc"
	}

	server := &Server{
		sdkServer: sdkmcp.NewServer(&sdkmcp.Implementation{Name: name, Version: options.Version}, nil),
		tools:     ToolInputSchemas(),
	}
	for toolName, schema := range server.tools {
		server.sdkServer.AddTool(&sdkmcp.Tool{
			Name:        toolName,
			Description: "wowdoc tool " + toolName,
			InputSchema: schema,
		}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{
				StructuredContent: map[string]any{"ok": false},
				IsError:           true,
			}, nil
		})
		server.sdkRegisteredTools++
	}
	return server
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

func (s *Server) SDKRegisteredToolCount() int {
	return s.sdkRegisteredTools
}
