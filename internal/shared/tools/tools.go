package tools

import (
	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
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
	Capabilities analyze.Capabilities `json:"capabilities"`
}

type ListClientsData struct {
	Clients []ClientSummary `json:"clients"`
}

type APIResult struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

func ListClients(repos []analyze.Repository, includeDiagnostics bool) contracts.Envelope[ListClientsData] {
	env := contracts.Envelope[ListClientsData]{OK: true}
	for _, repo := range repos {
		if repo.Valid {
			env.Data.Clients = append(env.Data.Clients, ClientSummary{
				Alias:        repo.Alias,
				Version:      repo.Version,
				Path:         repo.Path,
				Capabilities: repo.Capabilities,
			})
			continue
		}
		if includeDiagnostics {
			env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
		}
	}
	return env
}

func LookupBlizzardAPI(repo analyze.Repository, idx *analyze.Index, name string) contracts.Envelope[APIResult] {
	source := contracts.SourceTransparency{
		Client:  repo.Alias,
		Version: repo.Version,
		Path:    repo.Path,
	}
	if !repo.Capabilities.APIDocumentation {
		return contracts.Envelope[APIResult]{
			OK:     false,
			Source: source,
			Error: &contracts.ToolError{
				Code:    contracts.ErrCapabilityUnavailable,
				Message: "API documentation is unavailable for this source",
			},
		}
	}
	if idx != nil {
		if entry, ok := idx.APIs[name]; ok {
			return contracts.Envelope[APIResult]{
				OK:     true,
				Source: source,
				Data:   APIResult{Name: entry.Name, Type: entry.Type, Path: entry.Path},
			}
		}
	}
	return contracts.Envelope[APIResult]{OK: true, Source: source, Data: APIResult{Name: name}}
}
