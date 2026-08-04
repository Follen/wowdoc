package objectstore_test

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/objectstore"
)

func testLayout(t *testing.T) home.Layout {
	t.Helper()
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestPackStoresManyObjectsInOneFileAndReadsByHash(t *testing.T) {
	layout := testLayout(t)
	store := objectstore.New(layout, "fixture")
	first := []byte("function First()\n  return true\nend\n")
	second := []byte("function Second()\n  return false\nend\n")
	firstHash, _, reused, err := store.PutSource(first)
	if err != nil || reused {
		t.Fatalf("first reused=%v err=%v", reused, err)
	}
	secondHash, _, reused, err := store.PutSource(second)
	if err != nil || reused {
		t.Fatalf("second reused=%v err=%v", reused, err)
	}
	if _, _, reused, err = store.PutSource(first); err != nil || !reused {
		t.Fatalf("duplicate reused=%v err=%v", reused, err)
	}
	if err = store.Publish(); err != nil {
		t.Fatal(err)
	}
	packs, err := filepath.Glob(filepath.Join(layout.Packs, "*.pack"))
	if err != nil || len(packs) != 1 {
		t.Fatalf("packs=%v err=%v", packs, err)
	}
	for hash, want := range map[string][]byte{firstHash: first, secondHash: second} {
		reader, openErr := objectstore.Open(layout, objectstore.Source, hash)
		if openErr != nil {
			t.Fatal(openErr)
		}
		got, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil || string(got) != string(want) {
			t.Fatalf("hash=%s got=%q err=%v", hash, got, readErr)
		}
	}
}

func TestPublishedPackIsReusedByLaterBuild(t *testing.T) {
	layout := testLayout(t)
	data := []byte("return 'shared'\n")
	first := objectstore.New(layout, "first")
	hash, _, _, err := first.PutSource(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Publish(); err != nil {
		t.Fatal(err)
	}
	second := objectstore.New(layout, "second")
	if _, _, reused, putErr := second.PutSource(data); putErr != nil || !reused {
		t.Fatalf("reused=%v err=%v", reused, putErr)
	}
	if err = second.Publish(); err != nil {
		t.Fatal(err)
	}
	packs, _ := filepath.Glob(filepath.Join(layout.Packs, "*.pack"))
	if len(packs) != 1 || !objectstore.Exists(layout, objectstore.Source, hash) {
		t.Fatalf("packs=%v exists=%v", packs, objectstore.Exists(layout, objectstore.Source, hash))
	}
}

func TestConcurrentDuplicatePacksAreCompactedAtPublish(t *testing.T) {
	layout := testLayout(t)
	data := []byte("function ConcurrentDuplicate()\n  return true\nend\n")
	first := objectstore.New(layout, "first")
	second := objectstore.New(layout, "second")
	if _, _, reused, err := first.PutSource(data); err != nil || reused {
		t.Fatalf("first put reused=%v err=%v", reused, err)
	}
	if _, _, reused, err := second.PutSource(data); err != nil || reused {
		t.Fatalf("second put reused=%v err=%v", reused, err)
	}
	var wg sync.WaitGroup
	var firstErr, secondErr error
	wg.Add(2)
	go func() { defer wg.Done(); firstErr = first.Publish() }()
	go func() { defer wg.Done(); secondErr = second.Publish() }()
	wg.Wait()
	if firstErr != nil || secondErr != nil {
		t.Fatalf("publish first=%v second=%v", firstErr, secondErr)
	}
	packs, err := filepath.Glob(filepath.Join(layout.Packs, "*.pack"))
	if err != nil || len(packs) != 1 {
		t.Fatalf("packs=%v err=%v", packs, err)
	}
	reader, err := objectstore.Open(layout, objectstore.Source, objectstore.Hash(data))
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || string(got) != string(data) {
		t.Fatalf("got=%q err=%v", got, readErr)
	}
}

func TestLegacyRawObjectRemainsReadable(t *testing.T) {
	layout := testLayout(t)
	data := []byte("function Example()\n  return true\nend\n")
	hash := objectstore.Hash(data)
	path := filepath.Join(layout.Objects, hash[:2], hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := objectstore.Open(layout, objectstore.Source, hash)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(decoded) != string(data) {
		t.Fatalf("decoded=%q err=%v", decoded, err)
	}
}

func TestPackCorruptionIsDetected(t *testing.T) {
	layout := testLayout(t)
	data := []byte("return 'verified'\n")
	store := objectstore.New(layout, "corrupt")
	hash, _, _, err := store.PutSource(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Publish(); err != nil {
		t.Fatal(err)
	}
	packs, _ := filepath.Glob(filepath.Join(layout.Packs, "*.pack"))
	info, err := os.Stat(packs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Truncate(packs[0], info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if _, err = objectstore.Open(layout, objectstore.Source, hash); err == nil {
		t.Fatal("truncated pack was accepted")
	}
}

func TestAbortRemovesStagingPack(t *testing.T) {
	layout := testLayout(t)
	store := objectstore.New(layout, "abort")
	if _, _, _, err := store.PutSource([]byte("temporary")); err != nil {
		t.Fatal(err)
	}
	if err := store.Abort(); err != nil {
		t.Fatal(err)
	}
	staging, _ := filepath.Glob(filepath.Join(layout.Temp, "packs", "*"))
	if len(staging) != 0 {
		t.Fatalf("staging=%v", staging)
	}
}
