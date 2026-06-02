package analyze

import (
	"path/filepath"
	"testing"
)

func TestDetectRepositoryClassifiesValidPartialAndInvalidSources(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	valid := DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	if !valid.Valid || valid.Alias != "retail" || !valid.Capabilities.APIDocumentation || !valid.Capabilities.FrameXML {
		t.Fatalf("valid retail detection wrong: %#v", valid)
	}
	partial := DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	if !partial.Valid || partial.Capabilities.APIDocumentation || !partial.Capabilities.FrameXML {
		t.Fatalf("partial classic detection wrong: %#v", partial)
	}
	invalid := DetectRepository(filepath.Join(root, "invalid-random"), "random")
	if invalid.Valid || len(invalid.Diagnostics) == 0 {
		t.Fatalf("invalid directory must produce diagnostics: %#v", invalid)
	}
}
