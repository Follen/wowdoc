package main

import (
	"errors"
	"net/http"
	"testing"
)

func TestRunReturnsListenAndServeError(t *testing.T) {
	want := errors.New("bind failed")
	got := run(func(addr string, handler http.Handler) error {
		if addr == "" || handler == nil {
			t.Fatalf("server called with addr=%q handler=%v", addr, handler)
		}
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("run error = %v, want %v", got, want)
	}
}
