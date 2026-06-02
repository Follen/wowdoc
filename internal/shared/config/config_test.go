package config

import "testing"

func TestDefaultSeedsAreBootstrapDefaults(t *testing.T) {
	seeds := DefaultSourceSeeds()
	wantAliases := []string{"retail", "classic", "classic-ptr", "classic-titan", "ptr2"}
	if len(seeds) != len(wantAliases) {
		t.Fatalf("seed count = %d, want %d", len(seeds), len(wantAliases))
	}
	for i, alias := range wantAliases {
		if seeds[i].Alias != alias {
			t.Fatalf("seed %d alias = %q, want %q", i, seeds[i].Alias, alias)
		}
		if seeds[i].Repo == "" || seeds[i].Ref == "" {
			t.Fatalf("seed %q must include repo and ref: %#v", alias, seeds[i])
		}
	}
}
