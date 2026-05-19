package core

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"yuelaiengine/gateway/internal/plugin/httperr"
)

type webAssetServer struct {
	root string
}

func newWebAssetServer(root string) *webAssetServer {
	return &webAssetServer{root: root}
}

func (s *webAssetServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httperr.Write(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "仅支持 GET/HEAD")
		return
	}

	requestPath := strings.TrimSpace(r.URL.Path)
	if requestPath == "" {
		requestPath = "/web"
	}
	if requestPath == "/web" {
		requestPath = "/web/"
	}

	rel := strings.TrimPrefix(requestPath, "/web/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		rel = "index.html"
	}

	cleanRel := path.Clean("/" + rel)
	cleanRel = strings.TrimPrefix(cleanRel, "/")
	if cleanRel == "." || cleanRel == "" {
		cleanRel = "index.html"
	}

	target := filepath.Join(s.root, filepath.FromSlash(cleanRel))
	if isRegularFile(target) {
		http.ServeFile(w, r, target)
		return
	}

	if shouldFallbackToIndex(cleanRel) {
		indexPath := filepath.Join(s.root, "index.html")
		if isRegularFile(indexPath) {
			http.ServeFile(w, r, indexPath)
			return
		}
	}

	httperr.Write(w, http.StatusNotFound, "WEB_ASSET_NOT_FOUND", "前端资源不存在，请先构建 web/dist")
}

func shouldFallbackToIndex(rel string) bool {
	base := path.Base(rel)
	return !strings.Contains(base, ".")
}

func isRegularFile(filename string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (g *Gateway) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if g.webUI == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "WEB_UI_NOT_READY", "Web UI 未初始化")
		return
	}
	g.webUI.serve(w, r)
}
