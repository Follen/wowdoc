package result

import (
	"encoding/json"
	"fmt"
	"io"
)

type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
	NextSteps  []string       `json:"nextSteps,omitempty"`
	ExitStatus int            `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type Envelope struct {
	OK          bool           `json:"ok"`
	Data        any            `json:"data,omitempty"`
	Error       *Error         `json:"error,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
}

func E(code, message string, exit int) *Error {
	return &Error{Code: code, Message: message, ExitStatus: exit}
}

func Write(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(Envelope{OK: true, Data: data})
}

func WriteError(stdout, stderr io.Writer, err error) int {
	var appErr *Error
	if !As(err, &appErr) {
		appErr = E("internal_error", err.Error(), 1)
	}
	_ = json.NewEncoder(stdout).Encode(Envelope{OK: false, Error: appErr})
	fmt.Fprintln(stderr, appErr.Code+": "+appErr.Message)
	if appErr.ExitStatus > 0 {
		return appErr.ExitStatus
	}
	return 1
}

func As(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
