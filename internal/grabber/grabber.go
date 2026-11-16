// Package grabber scans a web page (and optionally the pages it links to) for
// downloadable files: the Site Grabber feature.
package grabber

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/KartikCodes1/forgedm/internal/config"
)

// Options control a scan.
type Options struct {
	URL       string            `json:"url"`
	Depth     int               `json:"depth"` // 0 = this page, n = follow links n levels, -1 = whole site
	SameSite  bool              `json:"sameSite"`
	MaxPages  int               `json:"maxPages"`
	Headers   map[string]string `json:"headers,omitempty"`
	UserAgent string            `json:"-"`
}

// Item is one file found.
type Item struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Type string `json:"type"`
	Ext  string `json:"ext"`
	Size int64  `json:"size"`
	Page string `json:"page"`
	Path string `json:"path"`
}

// Progress is reported while scanning.
type Progress struct {
	Pages   int    `json:"pages"`
	Found   int    `json:"found"`
	Sized   int    `json:"sized"`
	Stage   string `json:"stage"`
	Current string `json:"current"`
}

var pageExts = map[string]bool{"": true, "html": true, "htm": true, "php": true, "asp": true, "aspx": true, "jsp": true, "cgi": true, "shtml": true, "xhtml": true}
var skipExts = map[string]bool{"css": true, "js": true, "mjs": true, "json": true, "xml": true, "rss": true, "map": true, "woff": true, "woff2": true, "ttf": true, "otf": true, "eot": true}

func extOf(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), ".")
}

type crawler struct {
	opts   Options
	client *http.Client
	root   *url.URL
	mu     sync.Mutex
	seen   map[string]bool
	items  map[string]Item
	pages  int
}

// Scan crawls and returns the files it found, with sizes where the server says.
func Scan(ctx context.Context, client *http.Client, opts Options, progress func(Progress)) ([]Item, error) {
	root, err := url.Parse(strings.TrimSpace(opts.URL))
	if err != nil || (root.Scheme != "http" && root.Scheme != "https") {
		return nil, &url.Error{Op: "parse", URL: opts.URL, Err: errBadURL}
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 300
	}
	if progress == nil {
		progress = func(Progress) {}
	}
	c := &crawler{opts: opts, client: client, root: root, seen: map[string]bool{}, items: map[string]Item{}}

	type job struct {
		u     *url.URL
		depth int
	}
	level := []job{{root, 0}}
	c.seen[normalize(root)] = true
	maxDepth := opts.Depth
	if maxDepth < 0 {
		maxDepth = 50
	}
	for len(level) > 0 && ctx.Err() == nil {
		var next []job
		var nextMu sync.Mutex
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for _, j := range level {
			c.mu.Lock()
			if c.pages >= opts.MaxPages {
				c.mu.Unlock()
				break
			}
			c.pages++
			pages := c.pages
			found := len(c.items)
			c.mu.Unlock()
			progress(Progress{Pages: pages, Found: found, Stage: "pages", Current: j.u.String()})
			wg.Add(1)
			sem <- struct{}{}
			go func(j job) {
				defer wg.Done()
				defer func() { <-sem }()
				links := c.visit(ctx, j.u)
				if j.depth >= maxDepth {
					return
				}
				nextMu.Lock()
				for _, l := range links {
					next = append(next, job{l, j.depth + 1})
				}
				nextMu.Unlock()
			}(j)
		}
		wg.Wait()
		level = next
	}
	if ctx.Err() != nil {
		return c.list(), ctx.Err()
	}

	// Ask the server for sizes (first 600 files).
	items := c.list()
	limit := len(items)
	if limit > 600 {
		limit = 600
	}
	var sized int
	var smu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			items[i].Size = c.size(ctx, items[i].URL)
			smu.Lock()
			sized++
			n := sized
			smu.Unlock()
			if n%10 == 0 || n == limit {
				progress(Progress{Pages: c.pages, Found: len(items), Sized: n, Stage: "sizes"})
			}
		}(i)
	}
	wg.Wait()
	progress(Progress{Pages: c.pages, Found: len(items), Sized: limit, Stage: "done"})
	return items, ctx.Err()
}

type grabErr string

func (g grabErr) Error() string { return string(g) }

const errBadURL = grabErr("enter a web address that starts with http:// or https://")

func normalize(u *url.URL) string {
	c := *u
	c.Fragment = ""
	return c.String()
}

func (c *crawler) list() []Item {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Item, 0, len(c.items))
	for _, it := range c.items {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (c *crawler) sameSite(u *url.URL) bool {
	a := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	b := strings.TrimPrefix(strings.ToLower(c.root.Hostname()), "www.")
	return a == b || strings.HasSuffix(a, "."+b)
}

func (c *crawler) newRequest(ctx context.Context, method, u string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	for k, v := range c.opts.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// visit fetches one page, records files and returns pages to follow.
func (c *crawler) visit(ctx context.Context, u *url.URL) []*url.URL {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := c.newRequest(ctx, http.MethodGet, u.String())
	if err != nil {
		return nil
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "html") {
		// The start address itself is a file.
		c.addItem(resp.Request.URL, u.String(), "a")
		return nil
	}
	base := resp.Request.URL
	doc, err := html.Parse(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}
	var pages []*url.URL
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "base" {
				if h := attr(n, "href"); h != "" {
					if b, err := base.Parse(h); err == nil {
						base = b
					}
				}
			}
			for _, ref := range refsOf(n) {
				l, err := base.Parse(strings.TrimSpace(ref))
				if err != nil || (l.Scheme != "http" && l.Scheme != "https") {
					continue
				}
				l.Fragment = ""
				ext := extOf(l)
				if pageExts[ext] {
					if n.Data == "a" && (!c.opts.SameSite || c.sameSite(l)) {
						key := normalize(l)
						c.mu.Lock()
						if !c.seen[key] {
							c.seen[key] = true
							pages = append(pages, l)
						}
						c.mu.Unlock()
					}
					continue
				}
				if skipExts[ext] {
					continue
				}
				if c.opts.SameSite && !c.sameSite(l) && n.Data == "a" {
					// Files on other sites are still useful (CDNs); keep them.
				}
				c.addItem(l, u.String(), n.Data)
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return pages
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func refsOf(n *html.Node) []string {
	var out []string
	add := func(v string) {
		if v = strings.TrimSpace(v); v != "" && !strings.HasPrefix(v, "data:") && !strings.HasPrefix(v, "javascript:") && !strings.HasPrefix(v, "mailto:") {
			out = append(out, v)
		}
	}
	srcset := func(v string) {
		for _, part := range strings.Split(v, ",") {
			f := strings.Fields(strings.TrimSpace(part))
			if len(f) > 0 {
				add(f[0])
			}
		}
	}
	switch n.Data {
	case "a", "area":
		add(attr(n, "href"))
	case "img":
		add(attr(n, "src"))
		add(attr(n, "data-src"))
		add(attr(n, "data-original"))
		srcset(attr(n, "srcset"))
		srcset(attr(n, "data-srcset"))
	case "source":
		add(attr(n, "src"))
		srcset(attr(n, "srcset"))
	case "video", "audio", "embed", "track":
		add(attr(n, "src"))
	case "object":
		add(attr(n, "data"))
	}
	return out
}

func (c *crawler) addItem(l *url.URL, page, tag string) {
	name := path.Base(l.Path)
	if s, err := url.PathUnescape(name); err == nil {
		name = s
	}
	if name == "" || name == "/" || name == "." {
		return
	}
	key := normalize(l)
	ext := extOf(l)
	rel := strings.TrimPrefix(path.Dir(l.Path), "/")
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return
	}
	c.items[key] = Item{
		URL:  key,
		Name: name,
		Ext:  ext,
		Type: config.CategoryFor(name),
		Size: -1,
		Page: page,
		Path: filepath.FromSlash(rel),
	}
}

// size asks the server for the file size without downloading it.
func (c *crawler) size(ctx context.Context, u string) int64 {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := c.newRequest(ctx, http.MethodGet, u)
	if err != nil {
		return -1
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := c.client.Do(req)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusPartialContent {
		cr := resp.Header.Get("Content-Range")
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			if n, err := strconv.ParseInt(cr[i+1:], 10, 64); err == nil {
				return n
			}
		}
	}
	if resp.StatusCode == http.StatusOK {
		return resp.ContentLength
	}
	return -1
}
