package gitinstall

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/follenfang/wowdoc/internal/result"
)

type Command struct {
	Manager, Package string
	Steps            [][]string
	Interactive      bool
}

func Ensure(ctx context.Context, stderr io.Writer) (string, error) {
	if path, err := exec.LookPath("git"); err == nil {
		return verify(ctx, path)
	}
	plan, err := Detect(runtime.GOOS, exec.LookPath)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(stderr, "wowdata: Git is missing; installer=%s package=%s\n", plan.Manager, plan.Package)
	for _, step := range plan.Steps {
		fmt.Fprintf(stderr, "wowdata: running %s\n", strings.Join(step, " "))
		command := exec.CommandContext(ctx, step[0], step[1:]...)
		var captured bytes.Buffer
		stream := io.MultiWriter(stderr, &captured)
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, stream, stream
		if runErr := command.Run(); runErr != nil {
			return "", classifyInstallError(fmt.Errorf("%w: %s", runErr, strings.TrimSpace(captured.String())))
		}
	}
	if plan.Interactive {
		e := result.E("git_install_interactive", "the system Git installer requires user interaction", 4)
		e.NextSteps = []string{"finish the Xcode Command Line Tools dialog", "run git --version", "retry wowdata init"}
		return "", e
	}
	refreshPATH(runtime.GOOS)
	path, err := exec.LookPath("git")
	if err != nil {
		e := result.E("git_path_not_refreshed", "Git was installed but is not visible on PATH", 4)
		e.NextSteps = []string{"open a new terminal", "run git --version", "retry wowdata init"}
		return "", e
	}
	return verify(ctx, path)
}

func Detect(goos string, lookup func(string) (string, error)) (Command, error) {
	has := func(name string) bool { _, err := lookup(name); return err == nil }
	switch goos {
	case "windows":
		if has("winget") {
			return Command{Manager: "winget", Package: "Git.Git", Steps: [][]string{{"winget", "install", "--id", "Git.Git", "-e", "--source", "winget", "--accept-source-agreements", "--accept-package-agreements"}}}, nil
		}
		if has("scoop") {
			return Command{Manager: "scoop", Package: "git", Steps: [][]string{{"scoop", "install", "git"}}}, nil
		}
		if has("choco") {
			return Command{Manager: "choco", Package: "git", Steps: [][]string{{"choco", "install", "git", "-y"}}}, nil
		}
	case "darwin":
		if has("brew") {
			return Command{Manager: "brew", Package: "git", Steps: [][]string{{"brew", "install", "git"}}}, nil
		}
		if has("xcode-select") {
			return Command{Manager: "xcode-select", Package: "Command Line Tools", Steps: [][]string{{"xcode-select", "--install"}}, Interactive: true}, nil
		}
	case "linux":
		prefix := []string{}
		if os.Getenv("USER") != "root" && has("sudo") {
			prefix = []string{"sudo"}
		}
		step := func(command string, args ...string) []string {
			full := append([]string{}, prefix...)
			return append(full, append([]string{command}, args...)...)
		}
		if has("apt-get") {
			return Command{Manager: "apt", Package: "git", Steps: [][]string{step("apt-get", "update"), step("apt-get", "install", "-y", "git")}}, nil
		}
		if has("dnf") {
			return Command{Manager: "dnf", Package: "git", Steps: [][]string{step("dnf", "install", "-y", "git")}}, nil
		}
		if has("yum") {
			return Command{Manager: "yum", Package: "git", Steps: [][]string{step("yum", "install", "-y", "git")}}, nil
		}
		if has("pacman") {
			return Command{Manager: "pacman", Package: "git", Steps: [][]string{step("pacman", "-Sy", "--noconfirm", "git")}}, nil
		}
		if has("zypper") {
			return Command{Manager: "zypper", Package: "git", Steps: [][]string{step("zypper", "--non-interactive", "install", "git")}}, nil
		}
	}
	e := result.E("git_installer_not_found", "no supported Git package manager was found", 4)
	e.NextSteps = []string{"install Git from the operating system package manager", "run git --version", "retry wowdata init"}
	return Command{}, e
}

func verify(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		e := result.E("git_verification_failed", strings.TrimSpace(string(out)), 4)
		e.NextSteps = []string{"repair the Git installation", "run git --version", "retry wowdata init"}
		return "", e
	}
	version := strings.TrimSpace(string(out))
	if !strings.HasPrefix(version, "git version ") {
		return "", result.E("git_verification_failed", "unexpected git --version output", 4)
	}
	return version, nil
}
func refreshPATH(goos string) {
	var candidates []string
	if goos == "windows" {
		candidates = []string{filepath.Join(os.Getenv("ProgramFiles"), "Git", "cmd"), filepath.Join(os.Getenv("LocalAppData"), "Programs", "Git", "cmd")}
	} else if goos == "darwin" {
		candidates = []string{"/opt/homebrew/bin", "/usr/local/bin"}
	}
	path := os.Getenv("PATH")
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() && !strings.Contains(path, candidate) {
			path = candidate + string(os.PathListSeparator) + path
		}
	}
	_ = os.Setenv("PATH", path)
}
func classifyInstallError(err error) error {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "cancel"):
		e := result.E("git_install_cancelled", err.Error(), 4)
		e.NextSteps = []string{"rerun wowdata init when ready", "approve the operating system Git installation"}
		return e
	case strings.Contains(s, "access") || strings.Contains(s, "permission") || strings.Contains(s, "elevation"):
		e := result.E("git_install_admin_required", err.Error(), 4)
		e.NextSteps = []string{"run the package manager with administrator privileges", "run git --version", "retry wowdata init"}
		return e
	default:
		e := result.E("git_install_failed", err.Error(), 4)
		e.NextSteps = []string{"inspect the package manager output above", "install Git with the detected package manager", "run git --version", "retry wowdata init"}
		return e
	}
}
