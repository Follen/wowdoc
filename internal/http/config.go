package http

import (
	"os"

	sharedconfig "wowdoc/internal/shared/config"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig  `yaml:"server"`
	Sources  SourceConfig  `yaml:"sources"`
	Contexts ContextConfig `yaml:"contexts"`
	Limits   LimitConfig   `yaml:"limits"`
	Prepare  PrepareConfig `yaml:"prepare"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	BaseURL string `yaml:"base_url"`
}

type SourceConfig struct {
	Root              string        `yaml:"root"`
	AllowArbitraryRef bool          `yaml:"allow_arbitrary_ref"`
	DefaultRef        string        `yaml:"default_ref"`
	Defaults          []SourceEntry `yaml:"defaults"`
	Extra             []SourceEntry `yaml:"extra"`
}

type SourceEntry struct {
	Alias string `yaml:"alias"`
	Repo  string `yaml:"repo"`
	Ref   string `yaml:"ref"`
	Path  string `yaml:"path"`
}

type ContextConfig struct {
	MaxSourceContexts int      `yaml:"max_source_contexts"`
	MaxIndexContexts  int      `yaml:"max_index_contexts"`
	Pinned            []string `yaml:"pinned"`
}

type LimitConfig struct {
	MaxConcurrentSourceFetches int `yaml:"max_concurrent_source_fetches"`
	MaxConcurrentIndexBuilds   int `yaml:"max_concurrent_index_builds"`
	RequestTimeoutSeconds      int `yaml:"request_timeout_seconds"`
}

type PrepareConfig struct {
	PrewarmOnStart bool     `yaml:"prewarm_on_start"`
	PrewarmClients []string `yaml:"prewarm_clients"`
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 9789},
		Sources: SourceConfig{
			DefaultRef:        "latest",
			AllowArbitraryRef: false,
			Defaults:          defaultSourceEntries(),
		},
		Contexts: ContextConfig{MaxSourceContexts: 8, MaxIndexContexts: 4, Pinned: []string{"retail", "classic"}},
		Limits:   LimitConfig{MaxConcurrentSourceFetches: 2, MaxConcurrentIndexBuilds: 2, RequestTimeoutSeconds: 60},
	}
}

func defaultSourceEntries() []SourceEntry {
	seeds := sharedconfig.DefaultSourceSeeds()
	entries := make([]SourceEntry, 0, len(seeds))
	for _, seed := range seeds {
		entries = append(entries, SourceEntry{Alias: seed.Alias, Repo: seed.Repo, Ref: seed.Ref})
	}
	return entries
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
