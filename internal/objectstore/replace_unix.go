//go:build !windows

package objectstore

import "os"

func replaceFile(source, target string) error {
	return os.Rename(source, target)
}
