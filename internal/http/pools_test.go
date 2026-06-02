package http

import (
	"strconv"
	"sync"
	"testing"
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
