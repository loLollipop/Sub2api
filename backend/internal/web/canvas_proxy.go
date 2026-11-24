package web

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	infiniteCanvasPathPrefix = "/canvas"
	defaultCanvasStaticDir   = "/opt/infinite-canvas/html/canvas"
)

// canvasBootstrapScriptHashes are CSP source expressions for the two verified
// canvas bootstrap scripts. A hash only matches a byte-identical script body,
// so no other inline script gains execution.
var canvasBootstrapScriptHashes = []string{
	"'sha256-m0aYpSd9MsHzKnbbFe87yH92kbeFQYFIknWKtzL4O/g='",
	"'sha256-I6LPtG0ZaWWZjaqo01/h/CYOOBc9+Ljxd5XeZLu6aEI='",
}

// InfiniteCanvasHandler serves Infinite Canvas on the same origin as the
// Sub2API UI so the sidebar page does not jump hosts and browser calls to
// /v1 stay same-origin (no CORS). Prefer a local static dir; otherwise reverse
// proxy INFINITE_CANVAS_UPSTREAM.
func InfiniteCanvasHandler() gin.HandlerFunc {
	return newInfiniteCanvasHandler(bundledCanvasFS())
}

func newInfiniteCanvasHandler(bundled fs.FS) gin.HandlerFunc {
	staticDir := strings.TrimSpace(os.Getenv("INFINITE_CANVAS_STATIC_DIR"))
	if staticDir == "" {
		staticDir = defaultCanvasStaticDir
	}
	upstream := strings.TrimSpace(os.Getenv("INFINITE_CANVAS_UPSTREAM"))
	var proxy *httputil.ReverseProxy
	if parsed, err := parseCanvasUpstream(upstream); err != nil {
		if upstream != "" {
			fmt.Fprintf(os.Stderr, "infinite canvas upstream disabled: %v\n", err)
		}
	} else {
		// Operator-configured INFINITE_CANVAS_UPSTREAM; scheme/host already allowlisted.
		proxy = &httputil.ReverseProxy{
			Rewrite: func(req *httputil.ProxyRequest) {
				req.SetURL(parsed)
				req.Out.Host = parsed.Host
				// This proxy serves canvas assets; the external runtime must
				// never receive Sub2API login cookies or bearer credentials.
				req.Out.Header.Del("Cookie")
				req.Out.Header.Del("Authorization")
				req.Out.Header.Del("Referer")
			},
			ModifyResponse: func(res *http.Response) error {
				if res != nil {
					res.Header.Set("Referrer-Policy", "no-referrer")
					if strings.Contains(res.Header.Get("Content-Type"), "text/html") {
						res.Header.Set("Cache-Control", "no-store")
					}
					middleware.AllowSameOriginEmbedHeaders(res.Header)
					// Every CSP header is a separate policy, so extend each one
					// instead of removing the upstream policy.
					addCanvasScriptHashes(res.Header)
					allowCanvasLocalImageReads(res.Header)
				}
				return nil
			},
		}
	}

	fileServer := http.StripPrefix(infiniteCanvasPathPrefix, http.FileServer(http.Dir(staticDir)))

	return func(c *gin.Context) {
		if !isInfiniteCanvasPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		middleware.AllowSameOriginEmbedHeaders(c.Writer.Header())
		c.Header("Referrer-Policy", "no-referrer")
		addCanvasScriptHashes(c.Writer.Header())
		allowCanvasLocalImageReads(c.Writer.Header())
		if c.Request.URL.Path == infiniteCanvasPathPrefix {
			c.Redirect(http.StatusTemporaryRedirect, infiniteCanvasPathPrefix+"/")
			c.Abort()
			return
		}
		if serveInfiniteCanvasFile(c, staticDir, fileServer) {
			return
		}
		if serveBundledCanvas(c, bundled) {
			return
		}
		if proxy != nil {
			proxy.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusNotFound, "Infinite Canvas is not installed on this host")
		c.Abort()
	}
}

func serveBundledCanvas(c *gin.Context, assets fs.FS) bool {
	if assets == nil {
		return false
	}
	name := strings.TrimPrefix(c.Request.URL.Path, infiniteCanvasPathPrefix+"/")
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	content, err := fs.ReadFile(assets, name)
	if err != nil {
		// Client routes use the canvas index; a missing asset must stay a 404.
		if path.Ext(name) != "" {
			c.Status(http.StatusNotFound)
			c.Abort()
			return true
		}
		name = "index.html"
		content, err = fs.ReadFile(assets, name)
		if err != nil {
			return false
		}
	}
	if name == "index.html" {
		c.Header("Cache-Control", "no-store")
	}
	request := c.Request.Clone(c.Request.Context())
	request.URL.Path = "/" + name
	// ServeContent handles MIME types, HEAD and byte ranges without redirecting
	// index.html away from the iframe's /canvas/ prefix.
	http.ServeContent(c.Writer, request, name, time.Time{}, bytes.NewReader(content))
	c.Abort()
	return true
}

func isInfiniteCanvasPath(pathName string) bool {
	return pathName == infiniteCanvasPathPrefix || strings.HasPrefix(pathName, infiniteCanvasPathPrefix+"/")
}

func parseCanvasUpstream(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty upstream")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("invalid upstream host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("upstream must not contain query or fragment")
	}
	// Rebuild from allowlisted fields only so gosec does not treat env input as a live SSRF URL.
	return &url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: parsed.Path}, nil
}

func serveInfiniteCanvasFile(c *gin.Context, staticDir string, fileServer http.Handler) bool {
	if staticDir == "" {
		return false
	}
	staticRoot, err := filepath.Abs(staticDir)
	if err != nil {
		return false
	}
	info, err := os.Stat(staticRoot) // #nosec G703 -- root is an operator-configured asset directory, not a request path.
	if err != nil || !info.IsDir() {
		return false
	}
	rel := strings.TrimPrefix(c.Request.URL.Path, infiniteCanvasPathPrefix)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		rel = "index.html"
	}
	clean := path.Clean("/" + rel)
	full := filepath.Join(staticRoot, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	relative, err := filepath.Rel(staticRoot, full)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	if st, err := os.Stat(full); err == nil && !st.IsDir() { // #nosec G703 -- cleaned path is checked relative to staticRoot immediately above.
		if strings.HasSuffix(full, ".html") {
			c.Header("Cache-Control", "no-store")
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
		return true
	}
	index := filepath.Join(staticRoot, "index.html")
	if _, err := os.Stat(index); err != nil { // #nosec G703 -- fixed index.html beneath the operator-configured root.
		return false
	}
	c.Header("Cache-Control", "no-store")
	c.File(index)
	c.Abort()
	return true
}

// addCanvasScriptHashes extends every Content-Security-Policy value with the
// verified canvas bootstrap hashes. Multiple CSP headers intersect, so each
// policy is enhanced independently and none is removed. Both script-src and
// script-src-elem are covered: when script-src-elem is present it governs
// element scripts and needs its own hash sources.
func addCanvasScriptHashes(header http.Header) {
	if header == nil {
		return
	}
	values := header.Values("Content-Security-Policy")
	if len(values) == 0 {
		return
	}
	header.Del("Content-Security-Policy")
	for _, value := range values {
		header.Add("Content-Security-Policy", addCanvasScriptHashesToPolicy(value))
	}
}

func addCanvasScriptHashesToPolicy(policy string) string {
	segments := strings.Split(policy, ";")
	changed := false
	for i, segment := range segments {
		fields := strings.Fields(segment)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "script-src", "script-src-elem":
		default:
			continue
		}
		missing := make([]string, 0, len(canvasBootstrapScriptHashes))
		for _, hash := range canvasBootstrapScriptHashes {
			if !cspSourcesContain(fields[1:], hash) {
				missing = append(missing, hash)
			}
		}
		if len(missing) == 0 {
			continue
		}
		trimmed := strings.TrimRight(segment, " \t")
		segments[i] = trimmed + " " + strings.Join(missing, " ") + segment[len(trimmed):]
		changed = true
	}
	if !changed {
		return policy
	}
	return strings.Join(segments, ";")
}

func cspSourcesContain(sources []string, want string) bool {
	for _, source := range sources {
		if source == want {
			return true
		}
	}
	return false
}

// The original canvas reads its own base64/blob images with fetch before saving
// them to IndexedDB. Permit only these local schemes on canvas responses, while
// preserving every policy's existing network restrictions.
func allowCanvasLocalImageReads(header http.Header) {
	values := header.Values("Content-Security-Policy")
	header.Del("Content-Security-Policy")
	for _, value := range values {
		segments := strings.Split(value, ";")
		connect := -1
		var defaults []string
		for index, segment := range segments {
			fields := strings.Fields(segment)
			if len(fields) == 0 {
				continue
			}
			if strings.EqualFold(fields[0], "default-src") {
				defaults = fields[1:]
			}
			if strings.EqualFold(fields[0], "connect-src") {
				connect = index
			}
		}
		if connect < 0 && defaults == nil {
			header.Add("Content-Security-Policy", value)
			continue
		}
		sources := defaults
		if connect >= 0 {
			sources = strings.Fields(segments[connect])[1:]
		}
		filtered := make([]string, 0, len(sources)+2)
		for _, source := range sources {
			if source != "'none'" {
				filtered = append(filtered, source)
			}
		}
		for _, source := range []string{"blob:", "data:"} {
			if !cspSourcesContain(filtered, source) {
				filtered = append(filtered, source)
			}
		}
		directive := "connect-src " + strings.Join(filtered, " ")
		if connect >= 0 {
			segments[connect] = " " + directive
		} else {
			segments = append(segments, " "+directive)
		}
		header.Add("Content-Security-Policy", strings.Join(segments, ";"))
	}
}
