package http

type Config struct {
	Server   ServerConfig
	Sources  SourceConfig
	Contexts ContextConfig
	Limits   LimitConfig
}

type ServerConfig struct {
	Host    string
	Port    int
	BaseURL string
}

type SourceConfig struct {
	Root              string
	AllowArbitraryRef bool
	DefaultRef        string
}

type ContextConfig struct {
	MaxSourceContexts int
	MaxIndexContexts  int
	Pinned            []string
}

type LimitConfig struct {
	MaxConcurrentSourceFetches int
	MaxConcurrentIndexBuilds   int
	RequestTimeoutSeconds      int
}

func DefaultConfig() Config {
	return Config{
		Server:   ServerConfig{Host: "0.0.0.0", Port: 9789},
		Sources:  SourceConfig{DefaultRef: "latest", AllowArbitraryRef: false},
		Contexts: ContextConfig{MaxSourceContexts: 8, MaxIndexContexts: 4, Pinned: []string{"retail", "classic"}},
		Limits:   LimitConfig{MaxConcurrentSourceFetches: 2, MaxConcurrentIndexBuilds: 2, RequestTimeoutSeconds: 60},
	}
}
