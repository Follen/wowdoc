package query_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
)

// Includes many snapshot files, matching XML nodes and repeated call locations.
// A single-file fixture cannot expose the snapshot-files x XML scan regression.
func BenchmarkCompatibilityFrameReferences(b *testing.B) {
	layout, root := compatibilityFixture(b)
	for i := 0; i < 100; i++ {
		data := fmt.Sprintf("<Ui><Frame name=\"Frame%d\"/></Ui>", i)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("Frame%d.xml", i)), []byte(data), 0644); err != nil {
			b.Fatal(err)
		}
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "main", Commit: "9999999999999999999999999999999999999999", Input: indexer.DirectoryInput{Root: root}, Workers: 2})
	if err != nil {
		b.Fatal(err)
	}
	ctx := query.Context{SourceID: "fixture", ProductID: "main", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}
	usages := make([]query.CompatibilityUsage, 100)
	for i := range usages {
		usages[i] = query.CompatibilityUsage{Kind: "frame-type", Name: "Frame", File: "Addon.lua", Line: i + 1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		facts, unresolved, _, err := query.LookupCompatibility(layout, ctx, usages, "")
		if err != nil || len(facts) != len(usages) || len(unresolved) != 0 {
			b.Fatalf("facts=%d unresolved=%d err=%v", len(facts), len(unresolved), err)
		}
	}
}
