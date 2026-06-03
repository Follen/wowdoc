package analyze

import (
	"os"
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
		valid.Capabilities.Constants ||
		valid.Capabilities.Mixins ||
		!valid.Capabilities.CVars {
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

func TestDetectRepositoryDetectsConstantsOnlyFromConstantSignals(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	apiOnly := DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	if apiOnly.Capabilities.Constants {
		t.Fatalf("API documentation alone must not imply constants: %#v", apiOnly.Capabilities)
	}
	withConstants := DetectRepository(filepath.Join(root, "valid-retail-constants"), "retail-constants")
	if !withConstants.Valid || !withConstants.Capabilities.APIDocumentation ||
		!withConstants.Capabilities.WidgetDocs ||
		!withConstants.Capabilities.Constants {
		t.Fatalf("constant signal was not detected: %#v", withConstants)
	}
	if withConstants.Capabilities.FrameXML {
		t.Fatalf("generated API docs must not imply FrameXML: %#v", withConstants.Capabilities)
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

func TestDetectRepositoryInfersVersionFromRepositoryMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatalf("create source structure: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "build.info"), []byte("version=12.1.0.61000\n"), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	repo := DetectRepository(root, "metadata-retail")
	if !repo.Valid || repo.Version != "12.1.0.61000" {
		t.Fatalf("repository metadata version should make source valid: %#v", repo)
	}
}
