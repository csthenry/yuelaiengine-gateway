package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWebAssetServerServe(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatalf("write index failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "main.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write asset failed: %v", err)
	}

	srv := newWebAssetServer(root)

	t.Run("index fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/web/dashboard", nil)
		rec := httptest.NewRecorder()
		srv.serve(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	t.Run("serve static asset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/web/assets/main.js", nil)
		rec := httptest.NewRecorder()
		srv.serve(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	t.Run("asset not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/web/assets/missing.js", nil)
		rec := httptest.NewRecorder()
		srv.serve(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/web", nil)
		rec := httptest.NewRecorder()
		srv.serve(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
		}
	})
}
