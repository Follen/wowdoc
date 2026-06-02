package analyze

import (
	"path/filepath"
	"testing"
)

func TestBuildIndexFindsAPINamesInValidRetail(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "valid-retail"), "retail")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	api, ok := idx.APIs["C_AuctionHouse"]
	if !ok {
		t.Fatalf("expected C_AuctionHouse API namespace in index: %#v", idx.APIs)
	}
	if api.Name != "C_AuctionHouse" || api.Type != "System" {
		t.Fatalf("unexpected API namespace entry: %#v", api)
	}
}

func TestBuildIndexFindsSecureActionButtonTemplateFrameXMLInPartialClassic(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	repo := DetectRepository(filepath.Join(root, "partial-classic"), "classic")

	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	frame, ok := idx.FrameXML["SecureActionButtonTemplate"]
	if !ok {
		t.Fatalf("expected SecureActionButtonTemplate FrameXML in index: %#v", idx.FrameXML)
	}
	if frame.Name != "SecureActionButtonTemplate" || filepath.Base(frame.Path) != "SecureTemplates.lua" {
		t.Fatalf("unexpected FrameXML entry: %#v", frame)
	}
}
