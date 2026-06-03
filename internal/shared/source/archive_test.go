package source

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPArchiveFetcherDownloadsAndExtractsGitHubStyleZip(t *testing.T) {
	archive := githubStyleZip(t, map[string]string{
		"wow-ui-source-main/version.txt":                          "12.0.0.60000\n",
		"wow-ui-source-main/Interface/ui-code-list.txt":           "Interface/AddOns/Blizzard_APIDocumentationGenerated/GeneratedDocumentation.lua\n",
		"wow-ui-source-main/Interface/AddOns/.keep":               "",
		"wow-ui-source-main/Interface/AddOns/Blizzard_FrameXML/x": "",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/owner/wow-ui-source/archive/main.zip" {
			t.Fatalf("archive request path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "archives", "retail", "main")
	fetcher := NewHTTPArchiveFetcher(http.DefaultClient)
	if err := fetcher.FetchArchive(server.URL+"/owner/wow-ui-source.git", "main", destination); err != nil {
		t.Fatalf("FetchArchive failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destination, "version.txt")); err != nil {
		t.Fatalf("version.txt was not extracted at checkout root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "Interface", "ui-code-list.txt")); err != nil {
		t.Fatalf("Interface file was not extracted: %v", err)
	}
}

func TestHTTPArchiveFetcherFallsBackToGitHubBranchArchiveURL(t *testing.T) {
	archive := githubStyleZip(t, map[string]string{
		"wow-ui-source-feature/version.txt":                "12.0.0.60000\n",
		"wow-ui-source-feature/Interface/ui-code-list.txt": "",
	})
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/owner/wow-ui-source/archive/refs/heads/feature/auction-fix.zip" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "archives", "retail", "feature__auction-fix")
	fetcher := NewHTTPArchiveFetcher(server.Client())
	if err := fetcher.FetchArchive(server.URL+"/owner/wow-ui-source.git", "feature/auction-fix", destination); err != nil {
		t.Fatalf("FetchArchive failed: %v", err)
	}
	want := []string{
		"/owner/wow-ui-source/archive/feature/auction-fix.zip",
		"/owner/wow-ui-source/archive/refs/heads/feature/auction-fix.zip",
	}
	if len(paths) != len(want) {
		t.Fatalf("archive request paths = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("archive request paths = %#v, want %#v", paths, want)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "version.txt")); err != nil {
		t.Fatalf("version.txt was not extracted at checkout root: %v", err)
	}
}

func TestHTTPArchiveFetcherRejectsZipSlipEntries(t *testing.T) {
	archive := githubStyleZip(t, map[string]string{
		"wow-ui-source-main/../outside.txt": "bad",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "archives", "retail", "main")
	fetcher := NewHTTPArchiveFetcher(server.Client())
	err := fetcher.FetchArchive(server.URL+"/owner/wow-ui-source.git", "main", destination)
	if err == nil {
		t.Fatalf("expected zip slip archive to be rejected")
	}
}

func githubStyleZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
