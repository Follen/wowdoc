package source

import (
	"path/filepath"
	"testing"
)

func TestCheckoutPathIsIsolatedByClientAndResolvedCommit(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir()})
	got := m.CheckoutPath("classic", "1111111")
	want := filepath.Join(m.Root(), "checkouts", "classic", "1111111")
	if got != want {
		t.Fatalf("checkout path = %q, want %q", got, want)
	}
	other := m.CheckoutPath("classic", "2222222")
	if got == other {
		t.Fatalf("different commits must not share checkout path")
	}
}

func TestUnsupportedArbitraryRefIsRejectedWhenDisabled(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir(), AllowArbitraryRef: false, DefaultRefs: map[string]string{"retail": "main"}})
	_, err := m.ResolveRef("retail", "feature-branch")
	if err == nil || ErrorCode(err) != "unsupported_ref" {
		t.Fatalf("expected unsupported_ref, got %v", err)
	}
}
