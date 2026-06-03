package http

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolsKeyByClientAndResolvedCommit(t *testing.T) {
	p := NewPools(8, 4)
	p.PutSource("classic", "1111111", "source-a")
	p.PutSource("classic", "2222222", "source-b")
	if p.Source("classic", "1111111") == p.Source("classic", "2222222") {
		t.Fatalf("different commits must not share source context")
	}
}

func TestPoolsKeysDoNotCollideOnSeparator(t *testing.T) {
	p := NewPools(8, 4)
	p.PutSource("a@b", "c", "left")
	p.PutSource("a", "b@c", "right")
	if p.Source("a@b", "c") != "left" {
		t.Fatalf("left source was overwritten: %#v", p.Source("a@b", "c"))
	}
	if p.Source("a", "b@c") != "right" {
		t.Fatalf("right source was overwritten: %#v", p.Source("a", "b@c"))
	}
}

func TestPoolsSupportConcurrentAccess(t *testing.T) {
	p := NewPools(8, 4)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			commit := "commit" + strconv.Itoa(i)
			p.PutSource("retail", commit, i)
			_ = p.Source("retail", commit)
			_ = p.Stats()
		}(i)
	}
	wg.Wait()
}

func TestPoolsEvictLeastRecentlyUsedSources(t *testing.T) {
	p := NewPools(2, 2)
	p.PutSource("retail", "a", "source-a")
	p.PutSource("retail", "b", "source-b")
	if p.Source("retail", "a") != "source-a" {
		t.Fatalf("expected source-a before eviction")
	}
	p.PutSource("retail", "c", "source-c")

	if p.Source("retail", "a") != "source-a" {
		t.Fatalf("recently used source-a should be retained")
	}
	if p.Source("retail", "b") != nil {
		t.Fatalf("least recently used source-b should be evicted, got %#v", p.Source("retail", "b"))
	}
	if p.Source("retail", "c") != "source-c" {
		t.Fatalf("new source-c should be retained")
	}
	if stats := p.Stats(); stats["sources"] != 2 {
		t.Fatalf("source pool size = %d, want 2", stats["sources"])
	}
}

func TestPoolsEvictLeastRecentlyUsedIndexesSeparately(t *testing.T) {
	p := NewPools(1, 2)
	p.PutSource("retail", "a", "source-a")
	p.PutSource("retail", "b", "source-b")
	if p.Source("retail", "a") != nil {
		t.Fatalf("source pool should honor its own smaller capacity")
	}

	p.PutIndex("retail", "a", "api", "index-a")
	p.PutIndex("retail", "b", "api", "index-b")
	if p.Index("retail", "a", "api") != "index-a" {
		t.Fatalf("expected index-a before eviction")
	}
	p.PutIndex("retail", "c", "api", "index-c")

	if p.Index("retail", "a", "api") != "index-a" {
		t.Fatalf("recently used index-a should be retained")
	}
	if p.Index("retail", "b", "api") != nil {
		t.Fatalf("least recently used index-b should be evicted")
	}
	if stats := p.Stats(); stats["sources"] != 1 || stats["indexes"] != 2 {
		t.Fatalf("pool stats wrong: %#v", stats)
	}
}

func TestPoolsLoadSourceRunsOnceForConcurrentSameKey(t *testing.T) {
	p := NewPools(8, 4)
	var calls int32
	start := make(chan struct{})

	var wg sync.WaitGroup
	results := make(chan any, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := p.LoadSource(context.Background(), "retail", "abc123", func(context.Context) (any, error) {
				atomic.AddInt32(&calls, 1)
				<-start
				return "source", nil
			})
			if err != nil {
				t.Errorf("LoadSource returned error: %v", err)
				return
			}
			results <- value
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(start)
	wg.Wait()
	close(results)

	if calls != 1 {
		t.Fatalf("loader calls = %d, want 1", calls)
	}
	for value := range results {
		if value != "source" {
			t.Fatalf("loaded value = %#v, want source", value)
		}
	}
}

func TestPoolsLoadSourceRunsSeparatelyForDifferentKeys(t *testing.T) {
	p := NewPools(8, 4)
	var calls int32
	for _, commit := range []string{"a", "b"} {
		value, err := p.LoadSource(context.Background(), "retail", commit, func(context.Context) (any, error) {
			atomic.AddInt32(&calls, 1)
			return "source-" + commit, nil
		})
		if err != nil {
			t.Fatalf("LoadSource returned error: %v", err)
		}
		if value != "source-"+commit {
			t.Fatalf("value = %#v, want source-%s", value, commit)
		}
	}
	if calls != 2 {
		t.Fatalf("loader calls = %d, want 2", calls)
	}
}

func TestPoolsLoadIndexRunsOnceForConcurrentSameKey(t *testing.T) {
	p := NewPools(8, 4)
	var calls int32
	start := make(chan struct{})

	var wg sync.WaitGroup
	results := make(chan any, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := p.LoadIndex(context.Background(), "retail", "abc123", "api", func(context.Context) (any, error) {
				atomic.AddInt32(&calls, 1)
				<-start
				return "index", nil
			})
			if err != nil {
				t.Errorf("LoadIndex returned error: %v", err)
				return
			}
			results <- value
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(start)
	wg.Wait()
	close(results)

	if calls != 1 {
		t.Fatalf("loader calls = %d, want 1", calls)
	}
	for value := range results {
		if value != "index" {
			t.Fatalf("loaded value = %#v, want index", value)
		}
	}
}

func TestPoolsPreferEvictingUnpinnedSources(t *testing.T) {
	p := NewPoolsWithPinned(2, 2, []string{"retail"})
	p.PutSource("retail", "a", "pinned")
	p.PutSource("classic", "b", "unpinned")
	if p.Source("classic", "b") != "unpinned" {
		t.Fatalf("expected unpinned source before eviction")
	}
	p.PutSource("ptr", "c", "new")

	if p.Source("retail", "a") != "pinned" {
		t.Fatalf("pinned source should be retained")
	}
	if p.Source("classic", "b") != nil {
		t.Fatalf("unpinned source should be evicted before pinned")
	}
	if p.Source("ptr", "c") != "new" {
		t.Fatalf("new source should be retained")
	}
}

func TestPoolsPreferEvictingUnpinnedIndexes(t *testing.T) {
	p := NewPoolsWithPinned(2, 2, []string{"retail"})
	p.PutIndex("retail", "a", "api", "pinned-index")
	p.PutIndex("classic", "b", "api", "unpinned-index")
	if p.Index("classic", "b", "api") != "unpinned-index" {
		t.Fatalf("expected unpinned index before eviction")
	}
	p.PutIndex("ptr", "c", "api", "new-index")

	if p.Index("retail", "a", "api") != "pinned-index" {
		t.Fatalf("pinned index should be retained")
	}
	if p.Index("classic", "b", "api") != nil {
		t.Fatalf("unpinned index should be evicted before pinned")
	}
	if p.Index("ptr", "c", "api") != "new-index" {
		t.Fatalf("new index should be retained")
	}
}
