package config

type SourceSeed struct {
	Alias string `json:"alias" yaml:"alias"`
	Repo  string `json:"repo" yaml:"repo"`
	Ref   string `json:"ref" yaml:"ref"`
}

func DefaultSourceSeeds() []SourceSeed {
	return []SourceSeed{
		{Alias: "retail", Repo: "https://github.com/Gethe/wow-ui-source.git", Ref: "main"},
		{Alias: "classic", Repo: "https://github.com/Gethe/wow-ui-source-classic.git", Ref: "main"},
		{Alias: "classic-ptr", Repo: "https://github.com/Gethe/wow-ui-source-classic-ptr.git", Ref: "main"},
		{Alias: "classic-titan", Repo: "https://github.com/Gethe/wow-ui-source-classic-titan.git", Ref: "main"},
		{Alias: "ptr2", Repo: "https://github.com/Gethe/wow-ui-source-ptr2.git", Ref: "main"},
	}
}
