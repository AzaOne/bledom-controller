package server

import (
	"bytes"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type staticHandler struct {
	dir string
}

func newStaticHandler(dir string) http.Handler {
	return &staticHandler{dir: dir}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := cleanURLPath(r.URL.Path)
	if urlPath == "/" {
		urlPath = "/index.html"
	}

	ext := strings.ToLower(path.Ext(urlPath))
	switch ext {
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
	data, err := os.ReadFile(h.fsPathForURL(urlPath))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data = h.rewriteHTMLAssets(data, urlPath)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (h *staticHandler) serveJS(w http.ResponseWriter, r *http.Request, urlPath string) {
	data, err := os.ReadFile(h.fsPathForURL(urlPath))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data = h.rewriteJSImportSpecifiers(data, urlPath)

	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", cacheControlForJS(r, urlPath))
	_, _ = w.Write(data)
}

func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, urlPath, cacheControl string) {
	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	http.ServeFile(w, r, h.fsPathForURL(urlPath))
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
	info, err := os.Stat(h.fsPathForURL(urlPath))
	if err != nil {
		return "", false
	}
	return strconv.FormatInt(info.ModTime().Unix(), 10), true
}

func (h *staticHandler) fsPathForURL(urlPath string) string {
	clean := cleanURLPath(urlPath)
	clean = strings.TrimPrefix(clean, "/")
	return filepath.Join(h.dir, filepath.FromSlash(clean))
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

var htmlAssetRefRe = regexp.MustCompile(`(?i)(href|src)=(?:"(\.?/[^"?#]+\.(?:css|js))"|'(\.?/[^'?#]+\.(?:css|js))')`)
var jsImportRe = regexp.MustCompile(`(?m)\bimport\s+(?:[^;]*?\s+from\s+)?(?:"(\.?/[^"?#]+\.js)"|'(\.?/[^'?#]+\.js)')`)
var jsDynamicImportRe = regexp.MustCompile(`(?m)\bimport\(\s*(?:"(\.?/[^"?#]+\.js)"|'(\.?/[^'?#]+\.js)')\s*\)`)
