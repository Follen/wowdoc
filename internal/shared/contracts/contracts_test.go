package contracts

import "testing"

func TestErrorCodesAreStable(t *testing.T) {
	got := []ErrorCode{
		ErrClientRequired,
		ErrClientNotFound,
		ErrSourceNotFound,
		ErrSourceInvalid,
		ErrRefNotFound,
		ErrGitUnavailableArchiveFailed,
		ErrCapabilityUnavailable,
		ErrIndexUnavailable,
		ErrTimeout,
		ErrUnsupportedRef,
	}
	want := []string{
		"client_required",
		"client_not_found",
		"source_not_found",
		"source_invalid",
		"ref_not_found",
		"git_unavailable_archive_failed",
		"capability_unavailable",
		"index_unavailable",
		"timeout",
		"unsupported_ref",
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("error code %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToolEnvelopePreservesSourceErrorDiagnosticsAndData(t *testing.T) {
	env := Envelope[map[string]string]{
		OK: true,
		Source: SourceTransparency{
			Client: "retail", RequestedRef: "main", ResolvedRef: "abc123", Version: "12.0.0", Path: "sources/checkouts/retail/abc123",
		},
		Data:        map[string]string{"name": "C_AuctionHouse.GetItemSearchResultInfo"},
		Diagnostics: []Diagnostic{{Path: "bad", Message: "missing Interface"}},
	}
	if !env.OK || env.Source.Client != "retail" || env.Data["name"] == "" || len(env.Diagnostics) != 1 {
		t.Fatalf("envelope did not preserve fields: %#v", env)
	}
}
