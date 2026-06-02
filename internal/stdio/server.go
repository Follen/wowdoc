package stdio

import (
	"os"
	"path/filepath"
)

func DefaultSourceRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "sources"), nil
}
