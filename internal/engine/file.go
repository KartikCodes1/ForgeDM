package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KartikCodes1/forgedm/internal/platform"
)

var errRestart = errors.New("file changed on the server")

// request opens a GET for bytes [start, end] (end < 0 means open-ended).
func (e *Engine) request(ctx context.Context, d *Download, start, end int64, ranged bool, ifRange string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.URL, nil)
	if err != nil {
		return nil, &fatalError{err}
	}
	e.applyHeaders(req, d.Headers)
	if ranged {
		if end >= 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
		}
		if ifRange != "" {
			req.Header.Set("If-Range", ifRange)
		}
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, &httpError{Code: resp.StatusCode, Status: resp.Status}
	}
	return resp, nil
}

func (e *Engine) applyHeaders(req *http.Request, h map[string]string) {
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	e.mu.Lock()
	ua := e.cfg.UserAgent
	e.mu.Unlock()
	req.Header.Set("User-Agent", ua)
	for k, v := range h {
		if v == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "range", "if-range", "accept-encoding", "host", "content-length", "connection":
			continue
		}
		req.Header.Set(k, v)
	}
}

// parseContentRange reads "bytes a-b/total".
func parseContentRange(v string) (start, end, total int64, ok bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "bytes ") {
		return 0, 0, 0, false
	}
	v = strings.TrimPrefix(v, "bytes ")
	slash := strings.Index(v, "/")
	dash := strings.Index(v, "-")
	if slash < 0 || dash < 0 || dash > slash {
		return 0, 0, 0, false
	}
	var err error
	if start, err = strconv.ParseInt(v[:dash], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if end, err = strconv.ParseInt(v[dash+1:slash], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	t := v[slash+1:]
	if t == "*" {
		total = -1
	} else if total, err = strconv.ParseInt(t, 10, 64); err != nil {
		return 0, 0, 0, false
	}
	return start, end, total, true
}

func looksLikeHLS(u, contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "mpegurl") {
		return true
	}
	p := strings.ToLower(u)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(p, ".m3u8")
}

// Probe asks the server about a link without downloading it.
func (e *Engine) Probe(ctx context.Context, rawURL string, headers map[string]string) (ProbeResult, error) {
	d := &Download{URL: strings.TrimSpace(rawURL), Headers: headers}
	res := ProbeResult{URL: d.URL, Size: -1, Kind: KindFile}
	if looksLikeHLS(d.URL, "") {
		return e.probeHLS(ctx, d)
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	resp, err := e.request(ctx, d, 0, 0, true, "")
	if err != nil {
		var he *httpError
		if errors.As(err, &he) && he.Code == http.StatusRequestedRangeNotSatisfiable {
			res.Size = 0
			res.Filename = ResolveFilename("", d.URL, d.URL, "")
			return res, nil
		}
		return res, err
	}
	defer resp.Body.Close()
	res.FinalURL = resp.Request.URL.String()
	res.ContentType = resp.Header.Get("Content-Type")
	res.ETag = resp.Header.Get("ETag")
	res.LastMod = resp.Header.Get("Last-Modified")
	if looksLikeHLS(res.FinalURL, res.ContentType) {
		resp.Body.Close()
		return e.probeHLS(ctx, d)
	}
	if resp.StatusCode == http.StatusPartialContent {
		if _, _, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok {
			res.Size = total
			res.Resumable = total > 0
		}
	} else {
		res.Size = resp.ContentLength
		res.Resumable = false
		if strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") && res.Size > 0 {
			res.Resumable = true
		}
	}
	res.Filename = ResolveFilename(resp.Header.Get("Content-Disposition"), res.FinalURL, d.URL, res.ContentType)
	return res, nil
}

// ---- segment bookkeeping ----

// nextWork hands a connection its next piece: an unfinished idle segment if
// there is one, otherwise the second half of the biggest piece in progress.
func (t *task) nextWork(minSplit int64) *Segment {
	t.mu.Lock()
	defer t.mu.Unlock()
	d := t.d
	for _, s := range d.Segments {
		if !s.active && s.remaining() > 0 {
			s.active = true
			return s
		}
	}
	if !d.Resumable || d.Size <= 0 {
		return nil
	}
	var big *Segment
	for _, s := range d.Segments {
		if s.active && (big == nil || s.remaining() > big.remaining()) {
			big = s
		}
	}
	if big == nil || big.remaining() < 2*minSplit {
		return nil
	}
	rem := big.remaining()
	cur := big.Start + big.Done
	mid := cur + rem/2
	ns := &Segment{Start: mid, End: big.End, active: true}
	big.End = mid - 1
	d.Segments = append(d.Segments, ns)
	sort.Slice(d.Segments, func(i, j int) bool { return d.Segments[i].Start < d.Segments[j].Start })
	return ns
}

func (t *task) complete() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.d.Size < 0 {
		return false
	}
	var sum int64
	for _, s := range t.d.Segments {
		if s.remaining() > 0 {
			return false
		}
		sum += s.Done
	}
	return sum >= t.d.Size
}

// ---- the file runner ----

func (e *Engine) runFile(t *task, rs *runState) error {
	for attempt := 0; ; attempt++ {
		err := e.runFileOnce(t, rs)
		if errors.Is(err, errRestart) && attempt < 2 {
			t.mu.Lock()
			t.d.Segments = nil
			t.d.Downloaded = 0
			t.mu.Unlock()
			continue
		}
		if errors.Is(err, errRestart) {
			return &codedError{code: ErrCodeChanged, msg: "The file keeps changing on the server."}
		}
		return err
	}
}

func (e *Engine) runFileOnce(t *task, rs *runState) error {
	ctx := rs.ctx
	t.mu.Lock()
	d := t.d
	fresh := len(d.Segments) == 0
	if !fresh && !d.Resumable {
		// Without range support the only way to continue is to start over.
		d.Segments, d.Downloaded, fresh = nil, 0, true
	}
	t.mu.Unlock()

	var first *http.Response
	if fresh {
		resp, err := e.request(ctx, d, 0, -1, true, "")
		if err != nil {
			var he *httpError
			if errors.As(err, &he) && he.Code == http.StatusRequestedRangeNotSatisfiable {
				return e.finishEmpty(t)
			}
			return err
		}
		first = resp
		t.mu.Lock()
		d.FinalURL = resp.Request.URL.String()
		d.ContentType = resp.Header.Get("Content-Type")
		d.ETag = resp.Header.Get("ETag")
		d.LastMod = resp.Header.Get("Last-Modified")
		if resp.StatusCode == http.StatusPartialContent {
			if _, _, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok && total >= 0 {
				d.Size = total
				d.Resumable = true
			} else {
				d.Size = resp.ContentLength
				d.Resumable = false
			}
		} else {
			d.Size = resp.ContentLength
			d.Resumable = false
		}
		if !d.NameFixed {
			name := ResolveFilename(resp.Header.Get("Content-Disposition"), d.FinalURL, d.URL, d.ContentType)
			if name != d.Filename {
				t.mu.Unlock()
				reserved := e.reservedPaths(t)
				t.mu.Lock()
				d.Filename = UniqueName(d.Dir, name, reserved)
			}
		}
		end := int64(-1)
		if d.Size > 0 {
			end = d.Size - 1
		}
		d.Segments = []*Segment{{Start: 0, End: end}}
		d.Downloaded = 0
		t.mu.Unlock()
		e.markDirty()
		e.lis.Changed(t.snapshot())
		if d.Size == 0 {
			first.Body.Close()
			return e.finishEmpty(t)
		}
	}

	t.mu.Lock()
	dir, tempPath, size, resumable := d.Dir, d.TempPath(), d.Size, d.Resumable
	etag, lastMod, wantConns := d.ETag, d.LastMod, d.Connections
	t.mu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if first != nil {
			first.Body.Close()
		}
		return &fatalError{err}
	}
	flags := os.O_RDWR | os.O_CREATE
	if fresh {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(tempPath, flags, 0o644)
	if err != nil {
		if first != nil {
			first.Body.Close()
		}
		return &fatalError{err}
	}
	if fresh {
		platform.MakeSparse(f)
		if size > 0 {
			if err := f.Truncate(size); err != nil {
				f.Close()
				first.Body.Close()
				return &fatalError{err}
			}
		}
	} else if st, err := f.Stat(); err == nil && size > 0 && st.Size() != size {
		// The partial file was touched outside ForgeDM; start again.
		f.Close()
		return errRestart
	}

	ifRange := ""
	if !fresh {
		if etag != "" && !strings.HasPrefix(etag, "W/") {
			ifRange = etag
		} else if lastMod != "" {
			ifRange = lastMod
		}
	}

	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()
	conns := wantConns
	if conns <= 0 {
		conns = cfg.DefaultConns
	}
	if conns <= 0 {
		conns = AutoConnections(size)
	}
	if conns > cfg.MaxConns && cfg.MaxConns > 0 {
		conns = cfg.MaxConns
	}
	if conns > len(rs.wbytes) {
		conns = len(rs.wbytes)
	}
	if !resumable || size <= 0 {
		conns = 1
	}

	errs := make(chan error, conns)
	restart := make(chan struct{}, 1)
	var firstSeg *Segment
	if first != nil {
		t.mu.Lock()
		firstSeg = d.Segments[0]
		firstSeg.active = true
		t.mu.Unlock()
	}
	launched := 0
	launch := func(id int, seg *Segment, body io.ReadCloser) {
		launched++
		go func() {
			errs <- e.worker(t, rs, f, id, seg, body, ifRange, restart, cfg)
		}()
	}
	if firstSeg != nil {
		launch(0, firstSeg, first.Body)
	} else if s := t.nextWork(cfg.MinSplit); s != nil {
		launch(0, s, nil)
	}
	// Open the remaining connections gradually, like a browser would.
	for i := 1; i < conns; i++ {
		select {
		case <-ctx.Done():
		case <-time.After(150 * time.Millisecond):
		}
		if ctx.Err() != nil {
			break
		}
		s := t.nextWork(cfg.MinSplit)
		if s == nil {
			break
		}
		launch(i, s, nil)
	}

	var firstErr error
	for i := 0; i < launched; i++ {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	e.markDirty()

	select {
	case <-restart:
		return errRestart
	default:
	}
	if ctx.Err() != nil {
		if rs.failErr != nil {
			return rs.failErr
		}
		return context.Canceled
	}
	if firstErr != nil {
		return firstErr
	}
	if !t.complete() {
		if size < 0 {
			// Unknown size: the single stream ended, so that's the whole file.
			t.mu.Lock()
			d.Size = d.Downloaded
			t.mu.Unlock()
		} else {
			return &codedError{code: ErrCodeNetwork, msg: "The server closed the connection before the file was complete."}
		}
	}
	if syncErr != nil {
		return &fatalError{syncErr}
	}
	if closeErr != nil {
		return &fatalError{closeErr}
	}
	return e.finalize(t)
}

func (e *Engine) finishEmpty(t *task) error {
	t.mu.Lock()
	d := t.d
	t.mu.Unlock()
	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return &fatalError{err}
	}
	if err := os.WriteFile(d.TempPath(), nil, 0o644); err != nil {
		return &fatalError{err}
	}
	t.mu.Lock()
	d.Size, d.Downloaded = 0, 0
	t.mu.Unlock()
	return e.finalize(t)
}

// finalize moves the finished temp file to its real name.
func (e *Engine) finalize(t *task) error {
	t.mu.Lock()
	d := t.d
	src := d.TempPath()
	dst := d.Path()
	if _, err := os.Stat(dst); err == nil {
		t.mu.Unlock()
		reserved := e.reservedPaths(t)
		t.mu.Lock()
		d.Filename = UniqueName(d.Dir, d.Filename, reserved)
		dst = d.Path()
	}
	t.mu.Unlock()
	if err := os.Rename(src, dst); err != nil {
		return &fatalError{err}
	}
	t.mu.Lock()
	d.Downloaded = d.Size
	t.mu.Unlock()
	return nil
}

func (t *task) snapshot() Download {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.d.Snapshot()
}

// worker is one connection. It downloads its segment, then asks for more work
// until nothing worth splitting is left.
func (e *Engine) worker(t *task, rs *runState, f *os.File, id int, seg *Segment, body io.ReadCloser, ifRange string, restart chan struct{}, cfg Config) error {
	rs.conns.Add(1)
	defer rs.conns.Add(-1)
	retries := 0
	for seg != nil {
		err := e.fetch(t, rs, f, id, seg, body, ifRange, cfg)
		body = nil
		if err == nil {
			retries = 0
			t.mu.Lock()
			seg.active = false
			t.mu.Unlock()
			seg = t.nextWork(cfg.MinSplit)
			continue
		}
		if rs.ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, errRestart) {
			select {
			case restart <- struct{}{}:
			default:
			}
			rs.cancel()
			return nil
		}
		if isFatal(err) {
			rs.fail(err)
			return err
		}
		retries++
		if retries > cfg.RetryCount {
			rs.fail(err)
			return err
		}
		t.mu.Lock()
		if !t.d.Resumable {
			// Can't continue mid-file: rewind this stream to the start.
			t.d.Downloaded -= seg.Done
			seg.Done = 0
		}
		t.mu.Unlock()
		delay := cfg.RetryDelay << (retries - 1)
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		select {
		case <-rs.ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
	return nil
}

// fetch streams one segment to disk. It stops early when another connection
// has taken over the tail of this segment.
func (e *Engine) fetch(t *task, rs *runState, f *os.File, id int, seg *Segment, body io.ReadCloser, ifRange string, cfg Config) error {
	t.mu.Lock()
	start := seg.Start + seg.Done
	end := seg.End
	d := t.d
	resumable := d.Resumable
	t.mu.Unlock()
	if end >= 0 && start > end {
		if body != nil {
			body.Close()
		}
		return nil
	}

	reqCtx, cancelReq := context.WithCancel(rs.ctx)
	defer cancelReq()
	if body == nil {
		resp, err := e.request(reqCtx, d, start, end, resumable || start > 0, ifRange)
		if err != nil {
			var he *httpError
			if errors.As(err, &he) && he.Code == http.StatusRequestedRangeNotSatisfiable {
				return errRestart
			}
			return err
		}
		if resumable {
			if resp.StatusCode != http.StatusPartialContent {
				resp.Body.Close()
				return errRestart
			}
			if s, _, total, ok := parseContentRange(resp.Header.Get("Content-Range")); !ok || s != start || (total >= 0 && d.Size > 0 && total != d.Size) {
				resp.Body.Close()
				return errRestart
			}
		}
		body = resp.Body
	}
	defer body.Close()

	// Watchdog: a connection that goes quiet is dropped and retried.
	activity := make(chan struct{}, 1)
	go func() {
		idle := cfg.IdleTimeout
		if idle <= 0 {
			idle = 30 * time.Second
		}
		timer := time.NewTimer(idle)
		defer timer.Stop()
		for {
			select {
			case <-reqCtx.Done():
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idle)
			case <-timer.C:
				cancelReq()
				return
			}
		}
	}()

	buf := make([]byte, readChunk)
	for {
		if err := e.wait(rs, len(buf)); err != nil {
			return err
		}
		n, rerr := body.Read(buf)
		if n > 0 {
			select {
			case activity <- struct{}{}:
			default:
			}
			t.mu.Lock()
			pos := seg.Start + seg.Done
			allowed := int64(n)
			if seg.End >= 0 {
				if rem := seg.End - pos + 1; rem < allowed {
					allowed = rem
				}
			}
			if allowed > 0 {
				if _, werr := f.WriteAt(buf[:allowed], pos); werr != nil {
					t.mu.Unlock()
					return &fatalError{werr}
				}
				seg.Done += allowed
				d.Downloaded += allowed
			}
			finished := seg.End >= 0 && seg.Start+seg.Done > seg.End
			t.mu.Unlock()
			if allowed > 0 {
				rs.bytes.Add(allowed)
				rs.wbytes[id].Add(allowed)
				e.totalBytes.Add(allowed)
			}
			if finished {
				return nil
			}
		}
		if rerr == io.EOF {
			t.mu.Lock()
			unknown := seg.End < 0
			done := seg.End >= 0 && seg.Start+seg.Done > seg.End
			t.mu.Unlock()
			if unknown || done {
				return nil
			}
			return io.ErrUnexpectedEOF
		}
		if rerr != nil {
			if reqCtx.Err() != nil && rs.ctx.Err() == nil {
				return errors.New("connection timed out")
			}
			return rerr
		}
	}
}

func (e *Engine) wait(rs *runState, n int) error {
	if err := waitN(rs.ctx, e.global, n); err != nil {
		return err
	}
	if rs.qlimiter != nil {
		if err := waitN(rs.ctx, rs.qlimiter, n); err != nil {
			return err
		}
	}
	return waitN(rs.ctx, rs.limiter, n)
}

// tempDirFor returns the folder used for the pieces of an HLS download.
func tempDirFor(d *Download) string { return filepath.Clean(d.Path() + ".forgedm.parts") }
