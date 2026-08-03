package objectstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/follenfang/wowdoc/internal/home"
)

type Store struct{ Layout home.Layout }

func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (s Store) PutSource(data []byte) (string, string, bool, error) {
	return s.put(s.Layout.Objects, data)
}
func (s Store) PutAsset(data []byte) (string, string, bool, error) {
	return s.put(s.Layout.Assets, data)
}

func (s Store) put(root string, data []byte) (string, string, bool, error) {
	hash := Hash(data)
	path := filepath.Join(root, hash[:2], hash)
	if _, err := os.Stat(path); err == nil {
		return hash, path, true, nil
	}
	reused, err := writeOnce(path, data)
	if err != nil {
		return "", "", false, err
	}
	return hash, path, reused, nil
}

func (s Store) PutAST(schema, contentHash string, value any) (string, string, bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", "", false, err
	}
	hash := Hash(data)
	root := filepath.Join(s.Layout.AST, schema)
	path := filepath.Join(root, contentHash[:2], contentHash+".json")
	if _, err := os.Stat(path); err == nil {
		return hash, path, true, nil
	}
	reused, err := writeOnce(path, data)
	if err != nil {
		return "", "", false, err
	}
	return hash, path, reused, nil
}

func writeOnce(path string, data []byte) (bool, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return false, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func NormalizePath(path string) string {
	return strings.ToLower(strings.ReplaceAll(filepath.ToSlash(path), "/", "\\"))
}
