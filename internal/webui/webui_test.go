package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

// Every browser entry point and transitive module import must survive embedding
// in the single binary with the MIME types that browsers require for modules.
func TestEmbeddedAssetGraph(t *testing.T) {
	handler := Handler()
	links := regexp.MustCompile(`(?:src|href)="(/[^"\s]+)"`)
	imports := regexp.MustCompile(`from ['"](\./[^'"]+)['"]`)
	visited := make(map[string]bool)
	var visit func(string)
	visit = func(url string) {
		t.Helper()
		if visited[url] {
			return
		}
		visited[url] = true
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status %d", url, recorder.Code)
		}
		content := recorder.Body.String()
		if strings.HasSuffix(url, ".js") {
			if !strings.Contains(recorder.Header().Get("Content-Type"), "javascript") {
				t.Errorf("%s: invalid module MIME type", url)
			}
			for _, match := range imports.FindAllStringSubmatch(content, -1) {
				visit(path.Join(path.Dir(url), match[1]))
			}
		} else if url == "/" {
			for _, match := range links.FindAllStringSubmatch(content, -1) {
				visit(match[1])
			}
		}
	}
	visit("/")
	if len(visited) < 3 {
		t.Fatal("UI did not reference an external stylesheet and module entry point")
	}
	if err := fs.WalkDir(assets, "static", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && !visited["/"+strings.TrimPrefix(name, "static/")] && name != "static/index.html" {
			t.Errorf("embedded asset is unreachable: %s", name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
