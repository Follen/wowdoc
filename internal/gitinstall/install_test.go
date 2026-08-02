package gitinstall

import (
	"errors"
	"testing"

	"github.com/follenfang/wowdoc/internal/result"
)

func TestDetectWindowsPrefersWinget(t *testing.T) {
	plan, err := Detect("windows", func(name string) (string, error) {
		if name == "winget" || name == "choco" {
			return name, nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Manager != "winget" || plan.Package != "Git.Git" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
func TestDetectLinuxApt(t *testing.T) {
	t.Setenv("USER", "root")
	plan, err := Detect("linux", func(name string) (string, error) {
		if name == "apt-get" {
			return name, nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Manager != "apt" || len(plan.Steps) != 2 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
func TestDetectMissingManagerHasStableCode(t *testing.T) {
	_, err := Detect("windows", func(string) (string, error) { return "", errors.New("missing") })
	var appErr *result.Error
	if !result.As(err, &appErr) || appErr.Code != "git_installer_not_found" {
		t.Fatalf("unexpected error: %v", err)
	}
}
