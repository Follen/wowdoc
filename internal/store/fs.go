package store

import "os"

func mkdirAll(path string) error { return os.MkdirAll(path, 0o755) }
