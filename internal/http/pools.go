package http

import "sync"

type Pools struct {
	mu      sync.RWMutex
	sources map[poolKey]any
	indexes map[poolKey]any
}

type poolKey struct {
	client string
	commit string
}

func NewPools(maxSources, maxIndexes int) *Pools {
	_ = maxSources
	_ = maxIndexes
	return &Pools{sources: map[poolKey]any{}, indexes: map[poolKey]any{}}
}

func (p *Pools) PutSource(client, commit string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sources[poolKey{client: client, commit: commit}] = value
}

func (p *Pools) Source(client, commit string) any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sources[poolKey{client: client, commit: commit}]
}

func (p *Pools) Stats() map[string]int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return map[string]int{"sources": len(p.sources), "indexes": len(p.indexes)}
}
