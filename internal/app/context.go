package app

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/query"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/store"
)

type selection struct {
	layout  home.Layout
	cat     *store.Catalog
	source  catalog.Source
	product catalog.Product
	ctx     query.Context
}

func selectSnapshot(sourceID, productID, ref string) (selection, error) {
	layout, err := home.Resolve()
	if err != nil {
		return selection{}, err
	}
	if !layout.Initialized() {
		e := result.E("not_initialized", "wowdoc data is not initialized", 3)
		e.NextSteps = []string{"wowdata init"}
		return selection{}, e
	}
	source, ok := catalog.FindSource(sourceID)
	if !ok {
		return selection{}, result.E("source_not_found", "unknown source: "+sourceID, 2)
	}
	product, ok := catalog.FindProduct(source, productID)
	if !ok {
		return selection{}, result.E("unsupported_build", "source does not declare product: "+productID, 3)
	}
	cat, err := store.OpenCatalogRead(layout)
	if err != nil {
		return selection{}, err
	}
	commit, tag, err := cat.Resolve(source, product, ref)
	if err != nil {
		cat.Close()
		if err == sql.ErrNoRows {
			e := result.E("version_not_found", "no exact Tag matches version/ref: "+ref, 3)
			e.Details = map[string]any{"requestedVersion": ref, "sourceId": source.ID, "product": product.ID}
			e.NextSteps = []string{fmt.Sprintf("wowdoc source list --source %s --product %s", source.ID, product.ID)}
			return selection{}, e
		}
		if strings.Contains(err.Error(), "ambiguous") {
			return selection{}, result.E("ambiguous_version", err.Error(), 3)
		}
		return selection{}, err
	}
	snapshotID := source.ID + "-" + product.ID + "-" + commit
	snapshot, ready, snapshotErr := cat.Snapshot(source.ID, product.ID, commit)
	if snapshotErr != nil {
		cat.Close()
		return selection{}, snapshotErr
	}
	if !ready || snapshot.Status != "ready" {
		cat.Close()
		e := result.E("snapshot_not_ready", "the selected source snapshot is not indexed", 3)
		e.Details = map[string]any{"sourceId": source.ID, "product": product.ID, "requestedRef": ref, "resolvedCommit": commit}
		e.NextSteps = []string{fmt.Sprintf("wowdoc source sync --source %s --product %s", source.ID, product.ID), fmt.Sprintf("wowdoc index build --source %s --product %s --ref %s", source.ID, product.ID, ref)}
		return selection{}, e
	}
	return selection{layout: layout, cat: cat, source: source, product: product, ctx: query.Context{SourceID: source.ID, ProductID: product.ID, RequestedRef: ref, MatchedTag: tag, Commit: commit, SnapshotID: snapshotID}}, nil
}
