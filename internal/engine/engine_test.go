package engine

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fileServer struct {
	data       []byte
	perConn    int           // bytes per second per connection (0 = unlimited)
	noRange    bool          // behave like a server without range support
	dropEvery  int64         // cut a connection after this many bytes (0 = never)
	forbid     atomic.Bool   // answer 403 (expired link)
	requests   atomic.Int64
	maxActive  atomic.Int64
	active     atomic.Int64
	name       string
}

func (fs *fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fs.requests.Add(1)
	if fs.forbid.Load() {
		http.Error(w, "expired", http.StatusForbidden)
		return
	}
	n := fs.active.Add(1)
	defer fs.active.Add(-1)
	for {
		m := fs.maxActive.Load()
		if n <= m || fs.maxActive.CompareAndSwap(m, n) {
			break
		}
	}
	size := int64(len(fs.data))
	start, end := int64(0), size-1
	status := http.StatusOK
	if fs.name != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+fs.name+`"`)
	}
	w.Header().Set("ETag", `"v1"`)
	if rg := r.Header.Get("Range"); rg != "" && !fs.noRange {
		spec := strings.TrimPrefix(rg, "bytes=")
		parts := strings.SplitN(spec, "-", 2)
		start, _ = strconv.ParseInt(parts[0], 10, 64)
		if parts[1] != "" {
			end, _ = strconv.ParseInt(parts[1], 10, 64)
		}
		if start >= size {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if end >= size {
			end = size - 1
		}
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(status)
	chunk := int64(16 << 10)
	var sent int64
	for pos := start; pos <= end; pos += chunk {
		e := pos + chunk
		if e > end+1 {
			e = end + 1
		}
		if _, err := w.Write(fs.data[pos:e]); err != nil {
			return
		}
		sent += e - pos
		if fs.dropEvery > 0 && sent >= fs.dropEvery {
			return // simulate a dropped connection
		}
		if fs.perConn > 0 {
			time.Sleep(time.Duration(float64(e-pos) / float64(fs.perConn) * float64(time.Second)))
		}
	}
}

func randomData(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

type recorder struct {
	mu        sync.Mutex
	completed chan Download
}

func newRecorder() *recorder { return &recorder{completed: make(chan Download, 16)} }
func (r *recorder) Changed(Download)     {}
func (r *recorder) Removed(string)       {}
func (r *recorder) Completed(d Download) { r.completed <- d }

func testConfig() Config {
	c := DefaultConfig()
	c.RetryDelay = 50 * time.Millisecond
	c.MinSplit = 256 << 10
	c.IdleTimeout = 3 * time.Second
	return c
}

func waitDone(t *testing.T, rec *recorder, timeout time.Duration) Download {
	t.Helper()
	select {
	case d := <-rec.completed:
		return d
	case <-time.After(timeout):
		t.Fatalf("download did not finish within %v", timeout)
	}
	return Download{}
}

func checkFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content mismatch: got %d bytes (sha %x), want %d bytes (sha %x)", len(got), sha256.Sum256(got), len(want), sha256.Sum256(want))
	}
}

func TestSegmentedDownloadIsFasterAndCorrect(t *testing.T) {
	data := randomData(6 << 20)
	fs := &fileServer{data: data, perConn: 2 << 20, name: "big file.bin"}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dir := t.TempDir()
	rec := newRecorder()
	e, err := New(testConfig(), filepath.Join(dir, "downloads.json"), rec)
	if err != nil {
		t.Fatal(err)
	}
	e.Start()
	defer e.Close()
	start := time.Now()
	d, err := e.Add(Request{URL: srv.URL + "/files/get?id=1", Dir: dir, Connections: 8, Start: true})
	if err != nil {
		t.Fatal(err)
	}
	done := waitDone(t, rec, 30*time.Second)
	took := time.Since(start)
	if done.ID != d.ID || done.Filename != "big file.bin" {
		t.Fatalf("unexpected result %+v", done)
	}
	checkFile(t, filepath.Join(dir, "big file.bin"), data)
	// One connection would need ~3s at 2 MB/s; 8 connections should be well under that.
	if took > 2*time.Second {
		t.Errorf("segmented download took %v, expected parallel speed-up", took)
	}
	if fs.maxActive.Load() < 4 {
		t.Errorf("expected several parallel connections, saw %d", fs.maxActive.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "big file.bin.forgedm")); !os.IsNotExist(err) {
		t.Errorf("temp file left behind")
	}
}

func TestPauseResumeAcrossRestart(t *testing.T) {
	data := randomData(8 << 20)
	fs := &fileServer{data: data, perConn: 1 << 20}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dir := t.TempDir()
	store := filepath.Join(dir, "downloads.json")

	rec := newRecorder()
	e, _ := New(testConfig(), store, rec)
	e.Start()
	d, _ := e.Add(Request{URL: srv.URL + "/movie.mkv", Dir: dir, Connections: 4, Start: true})
	time.Sleep(1200 * time.Millisecond)
	e.Pause(d.ID)
	got, _ := e.Get(d.ID)
	if got.Status != StatusPaused || got.Downloaded <= 0 || got.Downloaded >= int64(len(data)) {
		t.Fatalf("after pause: status=%s downloaded=%d", got.Status, got.Downloaded)
	}
	partial := got.Downloaded
	e.Close()

	// A fresh engine picks the download back up from the saved state.
	// the server keeps throttling; 4 connections finish the rest quickly
	before := fs.requests.Load()
	rec2 := newRecorder()
	e2, err := New(testConfig(), store, rec2)
	if err != nil {
		t.Fatal(err)
	}
	e2.Start()
	defer e2.Close()
	e2.Resume(d.ID)
	waitDone(t, rec2, 30*time.Second)
	checkFile(t, filepath.Join(dir, "movie.mkv"), data)
	if fs.requests.Load() == before {
		t.Fatal("expected new requests after resume")
	}
	if partial == 0 {
		t.Fatal("expected partial progress before pause")
	}
}

func TestRecoversFromDroppedConnections(t *testing.T) {
	data := randomData(3 << 20)
	fs := &fileServer{data: data, dropEvery: 300 << 10}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dir := t.TempDir()
	rec := newRecorder()
	cfg := testConfig()
	cfg.RetryCount = 50
	e, _ := New(cfg, "", rec)
	e.Start()
	defer e.Close()
	_, err := e.Add(Request{URL: srv.URL + "/flaky.zip", Dir: dir, Connections: 4, Start: true})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, rec, 30*time.Second)
	checkFile(t, filepath.Join(dir, "flaky.zip"), data)
}

func TestServerWithoutRangeSupport(t *testing.T) {
	data := randomData(1<<20 + 123)
	fs := &fileServer{data: data, noRange: true}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dir := t.TempDir()
	rec := newRecorder()
	e, _ := New(testConfig(), "", rec)
	e.Start()
	defer e.Close()
	pr, err := e.Probe(context.Background(), srv.URL+"/plain.pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Resumable || pr.Size != int64(len(data)) {
		t.Fatalf("probe: %+v", pr)
	}
	e.Add(Request{URL: srv.URL + "/plain.pdf", Dir: dir, Connections: 8, Start: true})
	d := waitDone(t, rec, 20*time.Second)
	if d.Resumable {
		t.Error("should be marked not resumable")
	}
	checkFile(t, filepath.Join(dir, "plain.pdf"), data)
}

func TestExpiredLinkFailsWithCode(t *testing.T) {
	fs := &fileServer{data: randomData(1 << 20)}
	fs.forbid.Store(true)
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dir := t.TempDir()
	changed := make(chan Download, 32)
	lis := &funcListener{changed: func(d Download) { changed <- d }}
	e, _ := New(testConfig(), "", lis)
	e.Start()
	defer e.Close()
	d, _ := e.Add(Request{URL: srv.URL + "/x.iso", Dir: dir, Start: true})
	deadline := time.After(10 * time.Second)
	for {
		select {
		case c := <-changed:
			if c.ID == d.ID && c.Status == StatusFailed {
				if c.ErrorCode != ErrCodeLinkExpired {
					t.Fatalf("error code = %q", c.ErrorCode)
				}
				return
			}
		case <-deadline:
			t.Fatal("never failed")
		}
	}
}

type funcListener struct {
	changed func(Download)
}

func (f *funcListener) Changed(d Download) { f.changed(d) }
func (f *funcListener) Removed(string)     {}
func (f *funcListener) Completed(Download) {}

func TestQueueGateControlsStart(t *testing.T) {
	data := randomData(64 << 10)
	srv := httptest.NewServer(&fileServer{data: data})
	defer srv.Close()
	dir := t.TempDir()
	rec := newRecorder()
	e, _ := New(testConfig(), "", rec)
	e.SetGate("night", Gate{Active: false, Concurrent: 2})
	e.Start()
	defer e.Close()
	d, _ := e.Add(Request{URL: srv.URL + "/a.bin", Dir: dir, Queue: "night"})
	time.Sleep(700 * time.Millisecond)
	if got, _ := e.Get(d.ID); got.Status != StatusQueued {
		t.Fatalf("closed queue started a download: %s", got.Status)
	}
	e.SetGate("night", Gate{Active: true, Concurrent: 2})
	waitDone(t, rec, 10*time.Second)
	if !e.QueueIdle("night") {
		t.Error("queue should be idle")
	}
}

func TestSpeedLimit(t *testing.T) {
	data := randomData(1 << 20)
	srv := httptest.NewServer(&fileServer{data: data})
	defer srv.Close()
	dir := t.TempDir()
	rec := newRecorder()
	e, _ := New(testConfig(), "", rec)
	e.SetGlobalLimit(512 << 10)
	e.Start()
	defer e.Close()
	start := time.Now()
	e.Add(Request{URL: srv.URL + "/limited.bin", Dir: dir, Connections: 4, Start: true})
	waitDone(t, rec, 20*time.Second)
	if took := time.Since(start); took < 1200*time.Millisecond {
		t.Errorf("1 MB at 512 KB/s finished in %v; limit not applied", took)
	}
}

func TestFilenameHelpers(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="report.pdf"`:                  "report.pdf",
		`attachment; filename*=UTF-8''na%C3%AFve%20file.txt`: "naïve file.txt",
		`inline; filename=plain.zip`:                          "plain.zip",
	}
	for h, want := range cases {
		if got := FilenameFromDisposition(h); got != want {
			t.Errorf("%s => %q, want %q", h, got, want)
		}
	}
	if got := SanitizeFilename(`a<b>:c?.txt`); got != "a_b__c_.txt" {
		t.Errorf("sanitize = %q", got)
	}
	if got := SanitizeFilename("CON.txt"); got != "_CON.txt" {
		t.Errorf("reserved = %q", got)
	}
	if got := ResolveFilename("", "https://x.test/dl?id=5", "https://x.test/dl?id=5", "application/pdf"); got != "dl.pdf" {
		t.Errorf("resolve = %q", got)
	}
}

func TestHLSDownloadWithEncryption(t *testing.T) {
	key := []byte("0123456789abcdef")
	var plain [][]byte
	mux := http.NewServeMux()
	for i := 0; i < 6; i++ {
		seg := randomData(50000 + i*1000)
		plain = append(plain, seg)
		// AES-128-CBC with PKCS7 padding and IV = sequence number.
		pad := aes.BlockSize - len(seg)%aes.BlockSize
		enc := append(append([]byte{}, seg...), bytes.Repeat([]byte{byte(pad)}, pad)...)
		iv := make([]byte, 16)
		iv[15] = byte(i)
		block, _ := aes.NewCipher(key)
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(enc, enc)
		body := enc
		mux.HandleFunc(fmt.Sprintf("/v/seg%d.ts", i), func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	}
	mux.HandleFunc("/v/key.bin", func(w http.ResponseWriter, r *http.Request) { w.Write(key) })
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360\nv/low.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720\nv/media.m3u8\n")
	})
	mux.HandleFunc("/v/media.m3u8", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n")
		for i := 0; i < 6; i++ {
			fmt.Fprintf(&b, "#EXTINF:4.0,\nseg%d.ts\n", i)
		}
		b.WriteString("#EXT-X-ENDLIST\n")
		w.Write([]byte(b.String()))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	rec := newRecorder()
	e, _ := New(testConfig(), "", rec)
	e.Start()
	defer e.Close()
	pr, err := e.Probe(context.Background(), srv.URL+"/master.m3u8", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Kind != KindHLS || len(pr.Variants) != 2 || pr.Variants[0].Label != "720p" {
		t.Fatalf("probe: %+v", pr)
	}
	_, err = e.Add(Request{URL: srv.URL + "/master.m3u8", Kind: KindHLS, Filename: "Ocean life.ts", Dir: dir, Start: true})
	if err != nil {
		t.Fatal(err)
	}
	d := waitDone(t, rec, 20*time.Second)
	want := bytes.Join(plain, nil)
	checkFile(t, filepath.Join(dir, d.Filename), want)
	if d.Size != int64(len(want)) {
		t.Errorf("size = %d, want %d", d.Size, len(want))
	}
}

func TestDynamicSplitKeepsSegmentsConsistent(t *testing.T) {
	d := &Download{Size: 100 << 20, Resumable: true, Segments: []*Segment{{Start: 0, End: 100<<20 - 1, active: true}}}
	tk := &task{d: d}
	for i := 0; i < 15; i++ {
		if tk.nextWork(1<<20) == nil {
			t.Fatalf("split %d failed", i)
		}
	}
	var covered int64
	prevEnd := int64(-1)
	for _, s := range d.Segments {
		if s.Start != prevEnd+1 {
			t.Fatalf("gap or overlap at %d (prev end %d)", s.Start, prevEnd)
		}
		covered += s.End - s.Start + 1
		prevEnd = s.End
	}
	if covered != d.Size {
		t.Fatalf("covered %d of %d", covered, d.Size)
	}
}
