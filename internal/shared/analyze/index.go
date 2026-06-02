package analyze

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

type SearchResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type Index struct {
	APIs     map[string]APIEntry      `json:"apis"`
	FrameXML map[string]FrameXMLEntry `json:"frameXML"`
	Lines    []SearchResult           `json:"lines"`
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
			for i, line := range strings.Split(string(b), "\n") {
				idx.Lines = append(idx.Lines, SearchResult{File: path, Line: i + 1, Text: line})
			}
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

func (i *Index) SearchFrameXML(query string, limit int) []SearchResult {
	if limit <= 0 {
		return nil
	}
	var out []SearchResult
	for _, line := range i.Lines {
		if strings.Contains(line.Text, query) {
			out = append(out, line)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}
