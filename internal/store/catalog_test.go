package store_test

import (
	"testing"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/store"
)

func TestExactDisplayVersionToTag(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenCatalog(layout)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source, _ := catalog.FindSource("elvui")
	product, _ := catalog.FindProduct(source, "main")
	if err = db.Seed(catalog.Sources()); err != nil {
		t.Fatal(err)
	}
	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err = db.PublishRef(source.ID, product, commit, []store.TagRecord{{Name: "v15.18", Commit: commit, CommittedAt: 1}}); err != nil {
		t.Fatal(err)
	}
	resolved, tag, err := db.Resolve(source, product, "15.18")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != commit || tag != "v15.18" {
		t.Fatalf("resolved=%s tag=%s", resolved, tag)
	}
}
