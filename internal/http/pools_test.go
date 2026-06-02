package http

import "testing"

func TestPoolsKeyByClientAndResolvedCommit(t *testing.T) {
	p := NewPools(8, 4)
	p.PutSource("classic", "1111111", "source-a")
	p.PutSource("classic", "2222222", "source-b")
	if p.Source("classic", "1111111") == p.Source("classic", "2222222") {
		t.Fatalf("different commits must not share source context")
	}
}
