package http

import (
	"context"
	"sync"
)

type Pools struct {
	mu          sync.Mutex
	maxSources  int
	maxIndexes  int
	sources     map[poolKey]any
	indexes     map[indexPoolKey]any
	sourceLRU   []poolKey
	indexLRU    []indexPoolKey
	sourceLoads map[poolKey]*loadCall
	indexLoads  map[indexPoolKey]*loadCall
	pinned      map[string]bool
}

type poolKey struct {
	client string
	commit string
}

type indexPoolKey struct {
	client string
	commit string
	kind   string
}

func NewPools(maxSources, maxIndexes int) *Pools {
	return NewPoolsWithPinned(maxSources, maxIndexes, nil)
}

func NewPoolsWithPinned(maxSources, maxIndexes int, pinned []string) *Pools {
	pinnedMap := map[string]bool{}
	for _, client := range pinned {
		pinnedMap[client] = true
	}
	return &Pools{
		maxSources:  maxSources,
		maxIndexes:  maxIndexes,
		sources:     map[poolKey]any{},
		indexes:     map[indexPoolKey]any{},
		sourceLoads: map[poolKey]*loadCall{},
		indexLoads:  map[indexPoolKey]*loadCall{},
		pinned:      pinnedMap,
	}
}

type loadCall struct {
	done  chan struct{}
	value any
	err   error
}

func (p *Pools) PutSource(client, commit string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := poolKey{client: client, commit: commit}
	p.sources[key] = value
	p.sourceLRU = touchPoolKey(p.sourceLRU, key)
	p.evictSourcesLocked()
}

func (p *Pools) Source(client, commit string) any {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := poolKey{client: client, commit: commit}
	value := p.sources[key]
	if value != nil {
		p.sourceLRU = touchPoolKey(p.sourceLRU, key)
	}
	return value
}

func (p *Pools) LoadSource(ctx context.Context, client, commit string, load func(context.Context) (any, error)) (any, error) {
	key := poolKey{client: client, commit: commit}
	p.mu.Lock()
	if value := p.sources[key]; value != nil {
		p.sourceLRU = touchPoolKey(p.sourceLRU, key)
		p.mu.Unlock()
		return value, nil
	}
	if call := p.sourceLoads[key]; call != nil {
		p.mu.Unlock()
		select {
		case <-call.done:
			return call.value, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &loadCall{done: make(chan struct{})}
	p.sourceLoads[key] = call
	p.mu.Unlock()

	call.value, call.err = load(ctx)

	p.mu.Lock()
	delete(p.sourceLoads, key)
	if call.err == nil && call.value != nil {
		p.sources[key] = call.value
		p.sourceLRU = touchPoolKey(p.sourceLRU, key)
		p.evictSourcesLocked()
	}
	close(call.done)
	p.mu.Unlock()
	return call.value, call.err
}

func (p *Pools) PutIndex(client, commit, kind string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := indexPoolKey{client: client, commit: commit, kind: kind}
	p.indexes[key] = value
	p.indexLRU = touchIndexPoolKey(p.indexLRU, key)
	p.evictIndexesLocked()
}

func (p *Pools) Index(client, commit, kind string) any {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := indexPoolKey{client: client, commit: commit, kind: kind}
	value := p.indexes[key]
	if value != nil {
		p.indexLRU = touchIndexPoolKey(p.indexLRU, key)
	}
	return value
}

func (p *Pools) LoadIndex(ctx context.Context, client, commit, kind string, load func(context.Context) (any, error)) (any, error) {
	key := indexPoolKey{client: client, commit: commit, kind: kind}
	p.mu.Lock()
	if value := p.indexes[key]; value != nil {
		p.indexLRU = touchIndexPoolKey(p.indexLRU, key)
		p.mu.Unlock()
		return value, nil
	}
	if call := p.indexLoads[key]; call != nil {
		p.mu.Unlock()
		select {
		case <-call.done:
			return call.value, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &loadCall{done: make(chan struct{})}
	p.indexLoads[key] = call
	p.mu.Unlock()

	call.value, call.err = load(ctx)

	p.mu.Lock()
	delete(p.indexLoads, key)
	if call.err == nil && call.value != nil {
		p.indexes[key] = call.value
		p.indexLRU = touchIndexPoolKey(p.indexLRU, key)
		p.evictIndexesLocked()
	}
	close(call.done)
	p.mu.Unlock()
	return call.value, call.err
}

func (p *Pools) Stats() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]int{"sources": len(p.sources), "indexes": len(p.indexes)}
}

func touchPoolKey(keys []poolKey, key poolKey) []poolKey {
	for i, existing := range keys {
		if existing == key {
			copy(keys[i:], keys[i+1:])
			keys = keys[:len(keys)-1]
			break
		}
	}
	return append(keys, key)
}

func (p *Pools) evictSourcesLocked() {
	for p.maxSources > 0 && len(p.sources) > p.maxSources {
		idx := p.sourceEvictionIndexLocked()
		evict := p.sourceLRU[idx]
		p.sourceLRU = append(p.sourceLRU[:idx], p.sourceLRU[idx+1:]...)
		delete(p.sources, evict)
	}
}

func (p *Pools) sourceEvictionIndexLocked() int {
	for i, key := range p.sourceLRU {
		if !p.pinned[key.client] {
			return i
		}
	}
	return 0
}

func (p *Pools) evictIndexesLocked() {
	for p.maxIndexes > 0 && len(p.indexes) > p.maxIndexes {
		idx := p.indexEvictionIndexLocked()
		evict := p.indexLRU[idx]
		p.indexLRU = append(p.indexLRU[:idx], p.indexLRU[idx+1:]...)
		delete(p.indexes, evict)
	}
}

func (p *Pools) indexEvictionIndexLocked() int {
	for i, key := range p.indexLRU {
		if !p.pinned[key.client] {
			return i
		}
	}
	return 0
}

func touchIndexPoolKey(keys []indexPoolKey, key indexPoolKey) []indexPoolKey {
	for i, existing := range keys {
		if existing == key {
			copy(keys[i:], keys[i+1:])
			keys = keys[:len(keys)-1]
			break
		}
	}
	return append(keys, key)
}
