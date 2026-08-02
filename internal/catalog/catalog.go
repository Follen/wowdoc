package catalog

import "strings"

type Source struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Repository      string    `json:"repository"`
	Official        bool      `json:"official"`
	VersionPrefixes []string  `json:"versionPrefixes,omitempty"`
	Products        []Product `json:"products"`
}

type Product struct {
	ID       string   `json:"id"`
	Branch   string   `json:"branch"`
	Clients  []string `json:"clients,omitempty"`
	TagRules []string `json:"tagRules,omitempty"`
}

func Sources() []Source {
	return []Source{
		{ID: "wow-ui-source", Name: "World of Warcraft UI Source", Repository: "https://github.com/Gethe/wow-ui-source.git", Official: true, Products: []Product{
			{ID: "retail", Branch: "live", Clients: []string{"retail"}, TagRules: []string{`^(?:[6-9]|1[0-9])\.`}}, {ID: "ptr", Branch: "ptr", Clients: []string{"ptr"}, TagRules: []string{`^(?:[6-9]|1[0-9])\.`}},
			{ID: "ptr2", Branch: "ptr2", Clients: []string{"ptr2"}, TagRules: []string{`^(?:[6-9]|1[0-9])\.`}}, {ID: "beta", Branch: "beta", Clients: []string{"beta"}, TagRules: []string{`^(?:[6-9]|1[0-9])\.`}},
			{ID: "classic", Branch: "classic", Clients: []string{"classic"}, TagRules: []string{`^(?:2\.5|3\.4|4\.4|5\.5)\.`}}, {ID: "classic-ptr", Branch: "classic_ptr", Clients: []string{"classic-ptr"}, TagRules: []string{`^(?:2\.5|3\.4|4\.4|5\.5)\.`}},
			{ID: "classic-beta", Branch: "classic_beta", Clients: []string{"classic-beta"}, TagRules: []string{`^(?:2\.5|3\.4|4\.4|5\.5)\.`}}, {ID: "classic-era", Branch: "classic_era", Clients: []string{"classic-era", "era"}, TagRules: []string{`^1\.1[3-9]\.`}},
			{ID: "classic-era-ptr", Branch: "classic_era_ptr", Clients: []string{"classic-era-ptr"}, TagRules: []string{`^(?:1\.1[3-9]|2\.5)\.`}}, {ID: "anniversary", Branch: "classic_anniversary", Clients: []string{"anniversary"}, TagRules: []string{`^2\.5\.`}},
			{ID: "titan", Branch: "classic_titan", Clients: []string{"titan"}, TagRules: []string{`^3\.80\.`}},
		}},
		{ID: "elvui", Name: "ElvUI", Repository: "https://github.com/tukui-org/ElvUI.git", VersionPrefixes: []string{"v"}, Products: []Product{
			{ID: "main", Branch: "main"}, {ID: "ptr", Branch: "ptr"},
		}},
		{ID: "weakauras", Name: "WeakAuras2", Repository: "https://github.com/WeakAuras/WeakAuras2.git", Products: []Product{{ID: "main", Branch: "main"}}},
		{ID: "ndui", Name: "NDui", Repository: "https://github.com/siweia/NDui.git", Products: []Product{
			{ID: "main", Branch: "master", TagRules: []string{"^[0-9]"}}, {ID: "classic", Branch: "Classic", TagRules: []string{"^Classic-5"}},
			{ID: "era", Branch: "Era", TagRules: []string{"^[0-9]"}}, {ID: "anniversary", Branch: "Anniversary", TagRules: []string{"^Classic-2"}},
			{ID: "titan", Branch: "Titan", TagRules: []string{"^Classic-3"}},
		}},
		{ID: "ellesmereui", Name: "EllesmereUI", Repository: "https://github.com/EllesmereGaming/EllesmereUI.git", VersionPrefixes: []string{"v"}, Products: []Product{{ID: "main", Branch: "main"}}},
	}
}

func FindSource(id string) (Source, bool) {
	for _, source := range Sources() {
		if strings.EqualFold(source.ID, id) || strings.EqualFold(source.Name, id) {
			return source, true
		}
	}
	return Source{}, false
}

func FindProduct(source Source, id string) (Product, bool) {
	if id == "" && len(source.Products) == 1 {
		return source.Products[0], true
	}
	for _, product := range source.Products {
		if strings.EqualFold(product.ID, id) || strings.EqualFold(product.Branch, id) {
			return product, true
		}
		for _, client := range product.Clients {
			if strings.EqualFold(client, id) {
				return product, true
			}
		}
	}
	return Product{}, false
}
