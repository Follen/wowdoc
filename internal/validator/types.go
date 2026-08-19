package validator

type Evidence map[string]any

type Diagnostic struct {
	Severity string   `json:"severity"`
	Code     string   `json:"code"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	Message  string   `json:"message"`
	Evidence Evidence `json:"evidence"`
}

type Unresolved struct {
	Kind       string   `json:"kind"`
	Expression string   `json:"expression"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Column     int      `json:"column"`
	Reason     string   `json:"reason"`
	Evidence   Evidence `json:"evidence"`
}

type LoadSource struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
}

type LoadFile struct {
	Path      string       `json:"path"`
	Type      string       `json:"type"`
	LoadedBy  []LoadSource `json:"loadedBy"`
	LoadOrder int          `json:"loadOrder"`
}

type Closure struct {
	TOC         string            `json:"toc"`
	Interface   string            `json:"interface"`
	Files       []LoadFile        `json:"loadClosure"`
	Diagnostics []Diagnostic      `json:"diagnostics"`
	Contents    map[string][]byte `json:"-"`
}

type Usage struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Expression string `json:"expression,omitempty"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
}

type CompatibilityFact struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Exists    bool     `json:"exists"`
	Signature string   `json:"signature"`
	Evidence  Evidence `json:"evidence"`
}

type EvidenceLookup interface {
	Lookup(usages []Usage, interfaceValue string) ([]CompatibilityFact, []Unresolved, []Diagnostic, error)
}

type Coverage struct {
	Checked    int `json:"checked"`
	Resolved   int `json:"resolved"`
	Unresolved int `json:"unresolved"`
}

type Result struct {
	ID             string              `json:"id,omitempty"`
	Valid          bool                `json:"valid"`
	Path           string              `json:"path"`
	SourceID       string              `json:"sourceId"`
	Product        string              `json:"product"`
	Ref            string              `json:"ref"`
	RequestedRef   string              `json:"requestedRef"`
	MatchedTag     string              `json:"matchedTag"`
	ResolvedCommit string              `json:"resolvedCommit"`
	TOC            string              `json:"toc"`
	Interface      string              `json:"interface"`
	CheckedLua     int                 `json:"checkedLua"`
	CheckedXML     int                 `json:"checkedXml"`
	LoadClosure    []LoadFile          `json:"loadClosure"`
	Diagnostics    []Diagnostic        `json:"diagnostics"`
	Unresolved     []Unresolved        `json:"unresolved"`
	Coverage       Coverage            `json:"coverage"`
	Facts          []CompatibilityFact `json:"facts"`
}

type MatrixTarget struct {
	ID      string `json:"id"`
	TOC     string `json:"toc"`
	Source  string `json:"source,omitempty"`
	Product string `json:"product"`
	Ref     string `json:"ref"`
}

type MatrixConfig struct {
	Path    string         `json:"path"`
	Targets []MatrixTarget `json:"targets"`
}

type MatrixSummary struct {
	APIs                  FactSummary             `json:"apis"`
	Events                FactSummary             `json:"events"`
	XML                   FactSummary             `json:"xml"`
	Interfaces            map[string]any          `json:"interfaces"`
	SharedFiles           []string                `json:"sharedFiles"`
	TargetOnlyFiles       map[string][]string     `json:"targetOnlyFiles"`
	SharedDiagnostics     []Diagnostic            `json:"sharedDiagnostics"`
	TargetOnlyDiagnostics map[string][]Diagnostic `json:"targetOnlyDiagnostics"`
	Unresolved            map[string][]Unresolved `json:"unresolved"`
}

type FactDifference struct {
	Name    string               `json:"name"`
	Targets map[string]FactValue `json:"targets"`
}

type FactValue struct {
	Exists    bool   `json:"exists"`
	Signature string `json:"signature"`
}

type FactSummary struct {
	Shared      []string         `json:"shared"`
	Differences []FactDifference `json:"differences"`
}

type MatrixResult struct {
	Path    string        `json:"path"`
	Valid   bool          `json:"valid"`
	Targets []Result      `json:"targets"`
	Summary MatrixSummary `json:"summary"`
}
