package home

import (
	"os"
	"path/filepath"
)

type Layout struct {
	Root, Config, Repositories, Objects, Assets, AST, Indexes, State, Manifests, Locks, Logs, Temp string
}

func Resolve() (Layout, error) {
	root := os.Getenv("WOWDOC_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Layout{}, err
		}
		root = filepath.Join(home, ".wowdoc")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	return Layout{
		Root: root, Config: filepath.Join(root, "config"), Repositories: filepath.Join(root, "repositories"),
		Objects: filepath.Join(root, "objects", "source"), Assets: filepath.Join(root, "objects", "assets"),
		AST: filepath.Join(root, "ast"), Indexes: filepath.Join(root, "indexes"), State: filepath.Join(root, "state"),
		Manifests: filepath.Join(root, "manifests"), Locks: filepath.Join(root, "locks"),
		Logs: filepath.Join(root, "logs"), Temp: filepath.Join(root, "tmp"),
	}, nil
}

func (l Layout) Directories() []string {
	return []string{l.Root, l.Config, l.Repositories, l.Objects, l.Assets, l.AST, l.Indexes, l.State, l.Manifests, l.Locks, l.Logs, l.Temp}
}

func (l Layout) Ensure() error {
	for _, dir := range l.Directories() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (l Layout) Initialized() bool {
	info, err := os.Stat(filepath.Join(l.State, "catalog.sqlite"))
	return err == nil && !info.IsDir()
}
