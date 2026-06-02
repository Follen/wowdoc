package analyze

import (
	"path/filepath"
	"testing"
)

func TestDetectRepositoryClassifiesValidPartialAndInvalidSources(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	valid := DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	if !valid.Valid || valid.Alias != "retail" ||
		!valid.Capabilities.APIDocumentation ||
		!valid.Capabilities.FrameXML ||
		!valid.Capabilities.WidgetDocs ||
		!valid.Capabilities.Constants ||
		valid.Capabilities.Mixins ||
		valid.Capabilities.CVars {
		t.Fatalf("valid retail detection wrong: %#v", valid)
	}
	partial := DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	if !partial.Valid ||
		partial.Capabilities.APIDocumentation ||
		!partial.Capabilities.FrameXML ||
		partial.Capabilities.WidgetDocs ||
		partial.Capabilities.Constants ||
		!partial.Capabilities.Mixins ||
		partial.Capabilities.CVars {
		t.Fatalf("partial classic detection wrong: %#v", partial)
	}
	invalid := DetectRepository(filepath.Join(root, "invalid-random"), "random")
	if invalid.Valid || len(invalid.Diagnostics) == 0 {
		t.Fatalf("invalid directory must produce diagnostics: %#v", invalid)
	}
}

func TestDetectRepositoryDoesNotInferCapabilitiesFromAddOnsAlone(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-empty-addons"), "empty")
	if !repo.Valid {
		t.Fatalf("empty AddOns repository should be valid: %#v", repo)
	}
	if repo.Capabilities.APIDocumentation ||
		repo.Capabilities.FrameXML ||
		repo.Capabilities.WidgetDocs ||
		repo.Capabilities.Constants ||
		repo.Capabilities.Mixins ||
		repo.Capabilities.CVars {
		t.Fatalf("empty AddOns repository should not report capabilities: %#v", repo.Capabilities)
	}
}
