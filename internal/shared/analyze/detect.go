package analyze

import (
	"os"
	"path/filepath"

	"wowdoc/internal/shared/contracts"
)

type Capabilities struct {
	APIDocumentation bool `json:"apiDocumentation"`
	FrameXML         bool `json:"frameXML"`
	WidgetDocs       bool `json:"widgetDocs"`
	Constants        bool `json:"constants"`
	Mixins           bool `json:"mixins"`
	CVars            bool `json:"cvars"`
}

type Repository struct {
	Alias        string                 `json:"alias"`
	Path         string                 `json:"path"`
	Version      string                 `json:"version"`
	Valid        bool                   `json:"valid"`
	Capabilities Capabilities           `json:"capabilities"`
	Diagnostics  []contracts.Diagnostic `json:"diagnostics"`
}

func DetectRepository(path, alias string) Repository {
	repo := Repository{Alias: alias, Path: path}
	missing := make([]string, 0)
	if !exists(filepath.Join(path, "Interface")) {
		missing = append(missing, "Interface/")
	}
	if !hasAny(path,
		"Interface/ui-code-list.txt",
		"Interface/ui-toc-list.txt",
		"Interface/ui-gen-addon-list.txt",
		"Interface/AddOns",
	) {
		missing = append(missing, "source-list-or-addons")
	}
	versionPath := filepath.Join(path, "version.txt")
	if !exists(versionPath) {
		missing = append(missing, "version.txt")
	} else if b, err := os.ReadFile(versionPath); err == nil {
		repo.Version = stringTrim(b)
	}
	if len(missing) > 0 {
		repo.Diagnostics = append(repo.Diagnostics, contracts.Diagnostic{
			Path: path, Message: "source_invalid", Missing: missing,
		})
		return repo
	}
	repo.Valid = true
	addons := filepath.Join(path, "Interface", "AddOns")
	repo.Capabilities.APIDocumentation = exists(filepath.Join(addons, "Blizzard_APIDocumentationGenerated"))
	repo.Capabilities.FrameXML = exists(addons)
	repo.Capabilities.WidgetDocs = exists(filepath.Join(addons, "Blizzard_APIDocumentationGenerated"))
	repo.Capabilities.Constants = exists(filepath.Join(addons, "Blizzard_APIDocumentationGenerated"))
	repo.Capabilities.Mixins = exists(addons)
	repo.Capabilities.CVars = exists(addons)
	return repo
}

func hasAny(root string, rels ...string) bool {
	for _, rel := range rels {
		if exists(filepath.Join(root, rel)) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func stringTrim(b []byte) string {
	s := string(b)
	for len(s) > 0 {
		last := s[len(s)-1]
		if last != '\n' && last != '\r' && last != ' ' && last != '\t' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
