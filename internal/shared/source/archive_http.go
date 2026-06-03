package source

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HTTPArchiveFetcher struct {
	client *http.Client
}

func NewHTTPArchiveFetcher(client *http.Client) *HTTPArchiveFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPArchiveFetcher{client: client}
}

func (f *HTTPArchiveFetcher) FetchArchive(repoURL, ref, destination string) error {
	var lastErr error
	for _, url := range archiveURLs(repoURL, ref) {
		err := f.fetchArchiveURL(url, destination)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func (f *HTTPArchiveFetcher) fetchArchiveURL(url, destination string) error {
	resp, err := f.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("archive download returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	return extractArchive(reader.File, destination)
}

func archiveURL(repoURL, ref string) string {
	return archiveURLs(repoURL, ref)[0]
}

func archiveURLs(repoURL, ref string) []string {
	base := strings.TrimSuffix(repoURL, ".git")
	base = strings.TrimRight(base, "/")
	urls := []string{base + "/archive/" + ref + ".zip"}
	if strings.Contains(ref, "/") {
		urls = append(urls,
			base+"/archive/refs/heads/"+ref+".zip",
			base+"/archive/refs/tags/"+ref+".zip",
		)
	}
	return urls
}

func extractArchive(files []*zip.File, destination string) error {
	for _, file := range files {
		rel := stripArchiveRoot(file.Name)
		if rel == "" {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(rel))
		cleanDestination, err := filepath.Abs(destination)
		if err != nil {
			return err
		}
		cleanTarget, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if cleanTarget != cleanDestination && !strings.HasPrefix(cleanTarget, cleanDestination+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(file, target); err != nil {
			return err
		}
	}
	return nil
}

func stripArchiveRoot(name string) string {
	name = strings.TrimLeft(filepath.ToSlash(name), "/")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func extractFile(file *zip.File, target string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}
