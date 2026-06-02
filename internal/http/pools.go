package http

type Pools struct {
	sources map[string]any
	indexes map[string]any
}

func NewPools(maxSources, maxIndexes int) *Pools {
	_ = maxSources
	_ = maxIndexes
	return &Pools{sources: map[string]any{}, indexes: map[string]any{}}
}

func (p *Pools) PutSource(client, commit string, value any) {
	p.sources[client+"@"+commit] = value
}

func (p *Pools) Source(client, commit string) any {
	return p.sources[client+"@"+commit]
}

func (p *Pools) Stats() map[string]int {
	return map[string]int{"sources": len(p.sources), "indexes": len(p.indexes)}
}
