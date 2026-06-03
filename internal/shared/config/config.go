package config

type SourceSeed struct {
	Alias string `json:"alias" yaml:"alias"`
	Repo  string `json:"repo" yaml:"repo"`
	Ref   string `json:"ref" yaml:"ref"`
}

func DefaultSourceSeeds() []SourceSeed {
	repo := "https://github.com/Gethe/wow-ui-source.git"
	return []SourceSeed{
		{Alias: "retail", Repo: repo, Ref: "live"},
		{Alias: "classic", Repo: repo, Ref: "classic"},
		{Alias: "classic-ptr", Repo: repo, Ref: "classic_ptr"},
		{Alias: "classic-titan", Repo: repo, Ref: "classic_titan"},
		{Alias: "ptr", Repo: repo, Ref: "ptr2"},
		{Alias: "ptr2", Repo: repo, Ref: "ptr2"},
	}
}
