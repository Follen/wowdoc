package cli

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIDoesNotImportHTTP(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(filepath.Join(root, "internal", "cli"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			if imp.Path.Value == `"wowdoc/internal/http"` {
				t.Fatalf("CLI must not import HTTP server package: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
