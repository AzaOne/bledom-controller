package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"bledom-controller/internal/server/webassets"
)

type staticHandler struct {
	source *staticSource
}

type staticSource struct {
	fs              fs.FS
	mode            string
	location        string
	dynamicVersions bool
	versions        map[string]string
}

func newStaticHandler(overrideDir string) (http.Handler, string, error) {
	source, err := loadStaticSource(overrideDir)
	if err != nil {
		return nil, "", err
	}
	return &staticHandler{source: source}, source.description(), nil
}

func loadStaticSource(overrideDir string) (*staticSource, error) {
	if overrideDir != "" {
		if info, err := os.Stat(overrideDir); err == nil && info.IsDir() {
			return newStaticSource(os.DirFS(overrideDir), "external", overrideDir, true)
		}
		log.Printf("[Server] External web directory %q not found, falling back to embedded assets", overrideDir)
	}

	staticFS, err := webassets.FS()
	if err != nil {
		return nil, fmt.Errorf("load embedded web assets: %w", err)
	}
	return newStaticSource(staticFS, "embedded", "", false)
}

func newStaticSource(staticFS fs.FS, mode, location string, dynamicVersions bool) (*staticSource, error) {
	source := &staticSource{
		fs:              staticFS,
		mode:            mode,
		location:        location,
		dynamicVersions: dynamicVersions,
	}
	if dynamicVersions {
		return source, nil
	}

	versions, err := buildAssetVersions(staticFS)
	if err != nil {
		return nil, err
	}
	source.versions = versions
	return source, nil
}

func buildAssetVersions(staticFS fs.FS) (map[string]string, error) {
	versions := make(map[string]string)
	err := fs.WalkDir(staticFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(staticFS, name)
		if err != nil {
			return err
		}
		versions[cleanURLPath(name)] = hashBytes(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("index static assets: %w", err)
	}
	return versions, nil
}

func (s *staticSource) description() string {
	if s.mode == "external" {
		return fmt.Sprintf("external web directory %q", s.location)
	}
	return "embedded web assets"
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := cleanURLPath(r.URL.Path)
	if urlPath == "/" {
		urlPath = "/index.html"
	}

	switch strings.ToLower(path.Ext(urlPath)) {
	case ".html":
		h.serveHTML(w, r, urlPath)
	case ".js":
		h.serveJS(w, r, urlPath)
	case ".css":
		h.serveFile(w, r, urlPath, cacheControlForCSS(r))
	default:
		h.serveFile(w, r, urlPath, cacheControlForAsset(r))
	}
}

func (h *staticHandler) serveHTML(w http.ResponseWriter, r *http.Request, urlPath string) {
	data, err := h.readFile(urlPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data = h.rewriteHTMLAssets(data, urlPath)
	h.writeData(w, urlPath, data, "text/html; charset=utf-8", "no-cache")
}

func (h *staticHandler) serveJS(w http.ResponseWriter, r *http.Request, urlPath string) {
	data, err := h.readFile(urlPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data = h.rewriteJSImportSpecifiers(data, urlPath)
	h.writeData(w, urlPath, data, "application/javascript; charset=utf-8", cacheControlForJS(r, urlPath))
}

func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, urlPath, cacheControl string) {
	data, err := h.readFile(urlPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.writeData(w, urlPath, data, "", cacheControl)
}

func (h *staticHandler) rewriteHTMLAssets(content []byte, basePath string) []byte {
	baseDir := path.Dir(basePath)
	return htmlAssetRefRe.ReplaceAllFunc(content, func(match []byte) []byte {
		sub := htmlAssetRefRe.FindSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		refBytes := sub[2]
		if len(refBytes) == 0 && len(sub) > 3 {
			refBytes = sub[3]
		}
		if len(refBytes) == 0 {
			return match
		}
		ref := string(refBytes)
		if strings.Contains(ref, "?") || strings.Contains(ref, "#") {
			return match
		}
		resolved := resolveURL(baseDir, ref)
		version, ok := h.fileVersionFromURL(resolved)
		if !ok {
			return match
		}
		updated := ref + "?v=" + version
		return bytes.Replace(match, []byte(ref), []byte(updated), 1)
	})
}

func (h *staticHandler) rewriteJSImportSpecifiers(content []byte, basePath string) []byte {
	baseDir := path.Dir(basePath)
	out := jsImportRe.ReplaceAllFunc(content, func(match []byte) []byte {
		sub := jsImportRe.FindSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		refBytes := sub[1]
		if len(refBytes) == 0 && len(sub) > 2 {
			refBytes = sub[2]
		}
		if len(refBytes) == 0 {
			return match
		}
		ref := string(refBytes)
		updated := h.versionedJSImport(ref, baseDir)
		if updated == ref {
			return match
		}
		return bytes.Replace(match, []byte(ref), []byte(updated), 1)
	})

	out = jsDynamicImportRe.ReplaceAllFunc(out, func(match []byte) []byte {
		sub := jsDynamicImportRe.FindSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		refBytes := sub[1]
		if len(refBytes) == 0 && len(sub) > 2 {
			refBytes = sub[2]
		}
		if len(refBytes) == 0 {
			return match
		}
		ref := string(refBytes)
		updated := h.versionedJSImport(ref, baseDir)
		if updated == ref {
			return match
		}
		return bytes.Replace(match, []byte(ref), []byte(updated), 1)
	})

	return out
}

func (h *staticHandler) versionedJSImport(ref, baseDir string) string {
	if strings.Contains(ref, "?") || strings.Contains(ref, "#") {
		return ref
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "//") {
		return ref
	}
	resolved := resolveURL(baseDir, ref)
	version, ok := h.fileVersionFromURL(resolved)
	if !ok {
		return ref
	}
	return ref + "?v=" + version
}

func (h *staticHandler) fileVersionFromURL(urlPath string) (string, bool) {
	if h.source.dynamicVersions {
		data, err := h.readFile(urlPath)
		if err != nil {
			return "", false
		}
		return hashBytes(data), true
	}

	version, ok := h.source.versions[cleanURLPath(urlPath)]
	return version, ok
}

func (h *staticHandler) readFile(urlPath string) ([]byte, error) {
	return fs.ReadFile(h.source.fs, strings.TrimPrefix(cleanURLPath(urlPath), "/"))
}

func (h *staticHandler) writeData(w http.ResponseWriter, urlPath string, data []byte, contentType string, cacheControl string) {
	if contentType == "" {
		contentType = contentTypeForPath(urlPath, data)
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	w.Header().Set("ETag", `"`+hashBytes(data)+`"`)
	_, _ = w.Write(data)
}

func cacheControlForCSS(r *http.Request) string {
	if r.URL.Query().Get("v") != "" {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

func cacheControlForJS(r *http.Request, urlPath string) string {
	if strings.HasSuffix(urlPath, "/main.js") {
		return "no-cache"
	}
	if r.URL.Query().Get("v") != "" {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

func cacheControlForAsset(r *http.Request) string {
	if r.URL.Query().Get("v") != "" {
		return "public, max-age=31536000, immutable"
	}
	return ""
}

func resolveURL(baseDir, ref string) string {
	if strings.HasPrefix(ref, "/") {
		return cleanURLPath(ref)
	}
	return cleanURLPath(path.Join(baseDir, ref))
}

func cleanURLPath(p string) string {
	if p == "" {
		return "/"
	}
	return path.Clean("/" + p)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

func contentTypeForPath(urlPath string, data []byte) string {
	switch strings.ToLower(path.Ext(urlPath)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	}

	if contentType := mime.TypeByExtension(strings.ToLower(path.Ext(urlPath))); contentType != "" {
		return contentType
	}
	return http.DetectContentType(data)
}

var htmlAssetRefRe = regexp.MustCompile(`(?i)(href|src)=(?:"(\.?/[^"?#]+\.(?:css|js))"|'(\.?/[^'?#]+\.(?:css|js))')`)
var jsImportRe = regexp.MustCompile(`(?m)\bimport\s+(?:[^;]*?\s+from\s+)?(?:"(\.?/[^"?#]+\.js)"|'(\.?/[^'?#]+\.js)')`)
var jsDynamicImportRe = regexp.MustCompile(`(?m)\bimport\(\s*(?:"(\.?/[^"?#]+\.js)"|'(\.?/[^'?#]+\.js)')\s*\)`)
