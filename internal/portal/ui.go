package portal

import (
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"

	apicerberus "github.com/APICerberus/APICerebrus"
)

func embeddedPortalFS() (fs.FS, error) {
	return apicerberus.EmbeddedPortalFS()
}

func (s *Server) newPortalUIHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.uiFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(strings.TrimSpace(r.URL.Path), s.apiPrefix) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		cleanPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		requested, serveUI := s.resolvePortalAssetPath(cleanPath)
		if !serveUI {
			http.NotFound(w, r)
			return
		}

		if requested != "" && portalAssetExists(s.uiFS, requested) {
			cloned := r.Clone(r.Context())
			cloned.URL = cloneURL(r.URL)
			cloned.URL.Path = "/" + requested
			fileServer.ServeHTTP(w, cloned)
			return
		}

		index, err := fs.ReadFile(s.uiFS, "index.html")
		if err != nil {
			http.Error(w, "portal assets unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

func (s *Server) resolvePortalAssetPath(cleanPath string) (requested string, serveUI bool) {
	cleanPath = strings.TrimSpace(cleanPath)
	if cleanPath == "" || cleanPath == "." {
		cleanPath = "/"
	}

	// Vite assets are referenced as "/assets/*" with base "/".
	if strings.HasPrefix(cleanPath, "/assets/") || cleanPath == "/favicon.ico" {
		return strings.TrimPrefix(cleanPath, "/"), true
	}

	if s.pathPrefix == "" {
		return strings.TrimPrefix(cleanPath, "/"), true
	}

	if cleanPath == s.pathPrefix {
		return "", true
	}
	if strings.HasPrefix(cleanPath, s.pathPrefix+"/") {
		return strings.TrimPrefix(strings.TrimPrefix(cleanPath, s.pathPrefix), "/"), true
	}
	return "", false
}

func portalAssetExists(fileSystem fs.FS, name string) bool {
	if fileSystem == nil {
		return false
	}
	file, err := fileSystem.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func cloneURL(in *url.URL) *url.URL {
	if in == nil {
		return &url.URL{}
	}
	out := *in
	return &out
}
