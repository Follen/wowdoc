package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/follenfang/wowdoc/internal/result"
)

type Lock struct{ path string }

func Acquire(path, operation string, staleAfter time.Duration) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > staleAfter {
		_ = os.Remove(path)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			e := result.E("operation_in_progress", "another process owns this operation", 5)
			e.Details = map[string]any{"operation": operation, "lock": path, "retryAfterMs": 1000}
			return nil, e
		}
		return nil, err
	}
	_ = json.NewEncoder(file).Encode(map[string]any{"operation": operation, "pid": os.Getpid(), "startedAt": time.Now().UTC().Format(time.RFC3339)})
	_ = file.Close()
	return &Lock{path: path}, nil
}

func (l *Lock) Release() {
	if l != nil {
		_ = os.Remove(l.path)
	}
}
