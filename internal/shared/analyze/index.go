package analyze

import (
	"os"
	"path/filepath"
	"regexp"
)

type APIEntry struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

type FrameXMLEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Index struct {
	APIs     map[string]APIEntry      `json:"apis"`
	FrameXML map[string]FrameXMLEntry `json:"frameXML"`
}

var (
	apiDocPattern  = regexp.MustCompile(`Name\s*=\s*"([^"]+)"[^}]*Type\s*=\s*"([^"]+)"`)
	frameXMLSymbol = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*Template)\s*=`)
)

func BuildIndex(repo Repository) (*Index, error) {
	idx := &Index{
		APIs:     map[string]APIEntry{},
		FrameXML: map[string]FrameXMLEntry{},
	}
	if !repo.Valid {
		return idx, nil
	}
	err := filepath.WalkDir(repo.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".lua" && ext != ".xml" {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, match := range apiDocPattern.FindAllSubmatch(b, -1) {
			name := string(match[1])
			idx.APIs[name] = APIEntry{Name: name, Type: string(match[2]), Path: path}
		}
		if isFrameXMLSignal(path) {
			for _, match := range frameXMLSymbol.FindAllSubmatch(b, -1) {
				name := string(match[1])
				idx.FrameXML[name] = FrameXMLEntry{Name: name, Path: path}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return idx, nil
}
