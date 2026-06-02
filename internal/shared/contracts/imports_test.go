package contracts

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedPackagesDoNotImportRuntimeSurfaces(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(filepath.Join(root, "shared"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			for _, forbidden := range []string{`"wowdoc/internal/cli"`, `"wowdoc/internal/http"`, `"wowdoc/internal/stdio"`} {
				if imp.Path.Value == forbidden {
					t.Fatalf("shared package %s imports runtime package %s", path, forbidden)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
