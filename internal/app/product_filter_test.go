package app

import (
	"testing"

	"github.com/follenfang/wowdoc/internal/catalog"
)

// source sync validated --product only after mirroring whole sources, so an
// alias it did not understand paid a full network fetch before failing.
func TestProductKnownGatesSyncBeforeNetwork(t *testing.T) {
	sources := []catalog.Source{{
		ID: "wow-ui-source",
		Products: []catalog.Product{
			{ID: "classic-era", Branch: "classic_era", Clients: []string{"classic-era", "era"}},
			{ID: "forever", Branch: "forever", Clients: []string{"forever"}},
		},
	}}
	for _, id := range []string{"", "classic-era", "classic_era", "era", "ERA", "forever"} {
		if !productKnown(sources, id) {
			t.Errorf("productKnown(%q)=false, want true", id)
		}
	}
	for _, id := range []string{"retail", "er", "foreve"} {
		if productKnown(sources, id) {
			t.Errorf("productKnown(%q)=true, want false", id)
		}
	}
}

// syncSources gates on the shipped catalog, so assert against it directly.
func TestProductKnownAgainstShippedCatalog(t *testing.T) {
	sources := catalog.Sources()
	for _, id := range []string{"", "era", "classic-era", "classic_era", "forever", "retail", "titan"} {
		if !productKnown(sources, id) {
			t.Errorf("productKnown(%q)=false, want true", id)
		}
	}
	for _, id := range []string{"bogus", "er", "foreve", "classic-eraa"} {
		if productKnown(sources, id) {
			t.Errorf("productKnown(%q)=true, want false", id)
		}
	}
}

func TestInitAndSyncAgreeOnProductAliases(t *testing.T) {
	source := catalog.Source{ID: "wow-ui-source", Products: []catalog.Product{
		{ID: "classic-era", Branch: "classic_era", Clients: []string{"classic-era", "era"}},
		{ID: "forever", Branch: "forever", Clients: []string{"forever"}},
	}}
	// init selects with Product.Matches; sync gates with productKnown. Both must
	// accept exactly the same --product values.
	for _, id := range []string{"era", "classic_era", "classic-era", "forever", "retail"} {
		var initMatched bool
		for _, product := range source.Products {
			if product.Matches(id) {
				initMatched = true
			}
		}
		if initMatched != productKnown([]catalog.Source{source}, id) {
			t.Errorf("%q: init matched=%v but sync matched=%v", id, initMatched, !initMatched)
		}
	}
}
