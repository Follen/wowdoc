package catalog_test

import (
	"testing"

	"github.com/follenfang/wowdoc/internal/catalog"
)

func TestProductMatchesResolvesAliasesConsistently(t *testing.T) {
	classicEra := catalog.Product{ID: "classic-era", Branch: "classic_era", Clients: []string{"classic-era", "era"}}
	for _, id := range []string{"classic-era", "classic_era", "era", "CLASSIC-ERA", "classic-era "} {
		if !classicEra.Matches(id) {
			t.Errorf("Matches(%q)=false, want true", id)
		}
	}
	for _, id := range []string{"retail", "classic", "era2", "forever"} {
		if classicEra.Matches(id) {
			t.Errorf("Matches(%q)=true, want false", id)
		}
	}
	// An omitted filter means "every product", so callers can skip the empty case.
	if !classicEra.Matches("") || !classicEra.Matches("   ") {
		t.Error("an empty id must match every product")
	}
}

func TestFindProductKeepsEmptyIDUnambiguous(t *testing.T) {
	single := catalog.Source{ID: "weakauras", Products: []catalog.Product{{ID: "main", Branch: "main"}}}
	if product, ok := catalog.FindProduct(single, ""); !ok || product.ID != "main" {
		t.Fatalf("single-product source with empty id: %#v %v", product, ok)
	}
	multi := catalog.Source{ID: "wow-ui-source", Products: []catalog.Product{{ID: "retail", Branch: "live"}, {ID: "forever", Branch: "forever"}}}
	if _, ok := catalog.FindProduct(multi, ""); ok {
		t.Fatal("an empty id must stay ambiguous for a multi-product source")
	}
	if product, ok := catalog.FindProduct(multi, "forever"); !ok || product.ID != "forever" {
		t.Fatalf("FindProduct(forever)=%#v %v", product, ok)
	}
	if _, ok := catalog.FindProduct(multi, "nonexistent"); ok {
		t.Fatal("an unknown product id must not resolve")
	}
}

// Every product the shipped catalog declares must resolve by id, branch, and
// every client alias. This is the invariant the alias bug violated: commands
// disagreed about which --product values exist.
func TestShippedCatalogResolvesEveryDeclaredName(t *testing.T) {
	for _, source := range catalog.Sources() {
		for _, product := range source.Products {
			for _, name := range append([]string{product.ID, product.Branch}, product.Clients...) {
				resolved, ok := catalog.FindProduct(source, name)
				if !ok {
					t.Errorf("%s/%s: %q does not resolve", source.ID, product.ID, name)
					continue
				}
				if resolved.ID != product.ID {
					t.Errorf("%s: %q resolved to %q, want %q", source.ID, name, resolved.ID, product.ID)
				}
			}
		}
	}
}
