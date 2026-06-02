package contracts

type ErrorCode string

const (
	ErrClientRequired               ErrorCode = "client_required"
	ErrClientNotFound               ErrorCode = "client_not_found"
	ErrSourceNotFound               ErrorCode = "source_not_found"
	ErrSourceInvalid                ErrorCode = "source_invalid"
	ErrRefNotFound                  ErrorCode = "ref_not_found"
	ErrGitUnavailableArchiveFailed ErrorCode = "git_unavailable_archive_failed"
	ErrCapabilityUnavailable       ErrorCode = "capability_unavailable"
	ErrIndexUnavailable            ErrorCode = "index_unavailable"
	ErrTimeout                     ErrorCode = "timeout"
	ErrUnsupportedRef              ErrorCode = "unsupported_ref"
)

type SourceTransparency struct {
	Client       string `json:"client"`
	RequestedRef string `json:"requestedRef,omitempty"`
	ResolvedRef  string `json:"resolvedRef,omitempty"`
	Version      string `json:"version,omitempty"`
	Path         string `json:"path,omitempty"`
}

type ToolError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type Diagnostic struct {
	Path    string   `json:"path,omitempty"`
	Message string   `json:"message"`
	Missing []string `json:"missing,omitempty"`
}

type Envelope[T any] struct {
	OK          bool               `json:"ok"`
	Source      SourceTransparency `json:"source,omitempty"`
	Data        T                  `json:"data,omitempty"`
	Error       *ToolError         `json:"error,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}
